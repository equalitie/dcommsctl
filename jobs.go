package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// this is a file for handling jobs: whatever client requested us to do and we essentially run it in async mode

type JobID string

type JobState string

const (
	JobPending JobState = "pending"
	JobRunning JobState = "running"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

type JobStatus struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`        // install, uninstall, start, ...
	InstanceID   string   `json:"instance_id"` // "" for jobs not tied to an instance
	State        JobState `json:"state"`
	ExitCode     int      `json:"exit_code"`     // meaningful once State is done/failed
	Error        string   `json:"error"`         // human-readable failure reason
	BytesWritten int      `json:"bytes_written"` // current log size; lets clients pace polling
	StartedAt    string   `json:"started_at"`    // RFC3339
	FinishedAt   string   `json:"finished_at"`   // RFC3339, empty while running
}

type JobRecord struct {
	Status       JobStatus
	ctx          context.Context
	cancel       context.CancelFunc
	LockFilePath string `json:"lock_file_path"`
	LogFilePath  string `json:"log_file_path"`
}

type JobManager struct {
	mu sync.RWMutex

	jobs map[string]*JobRecord

	jobsDBpath string
}

func isJobActive(s JobState) bool {
	return s == JobPending || s == JobRunning
}

func jobID(jobType string) string {
	return fmt.Sprintf(
		"%s-%s",
		jobType,
		time.Now().UTC().Format("01-02.15:04:05"),
	)
}

func jobLockFile(lockPath string) *os.File {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return f
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*JobRecord),
	}
}

func (m *JobManager) loadJobs() error {
	data, err := os.ReadFile(m.jobsDBpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// did not find the file, move on
			return nil
		}
		return err
	}

	var tmp map[string]*JobRecord

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return fmt.Errorf("jobs_db file unmarshal error %s. Possibly old format", err)
	}

	m.mu.Lock()
	m.jobs = tmp
	m.mu.Unlock()

	return nil
}

func (m *JobManager) dumpJobs() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := json.MarshalIndent(m.jobs, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(m.jobsDBpath, d, 0o644); err != nil {
		return err
	}

	return nil
}

// statusByID looks up a job by its bare id (the value handed back to clients as
// job_id), independent of which instance it belongs to. The jobs map is keyed
// by "instanceID-jobID", but a caller polling job/status only has the bare id.
func (m *JobManager) statusByID(id string) (JobStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rec := range m.jobs {
		if rec.Status.ID == id {
			return rec.Status, true
		}
	}

	return JobStatus{}, false
}

func (m *JobManager) fetchLog(instanceID string, jobID string, offset int64) ([]byte, error) {
	rec, found := m.jobs[instanceID+"-"+jobID]

	if !found {
		return nil, fmt.Errorf("Job not found for instance '%s' and job id '%s'", instanceID, jobID)
	}

	f, err := os.OpenFile(rec.LogFilePath, os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	length := int64(64 * 1024)

	if offset >= stat.Size() {
		return nil, nil
	}

	if offset+length > stat.Size() {
		length = stat.Size() - offset
	}

	buf := make([]byte, length)

	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return buf[:n], nil
}

func (m *JobManager) run(rec *JobRecord, cmd *exec.Cmd) error {
	jobID := rec.Status.ID
	f, err := os.OpenFile(rec.LogFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	wout := io.MultiWriter(f, os.Stdout)
	werr := io.MultiWriter(f, os.Stderr)

	cmd.Stdout = wout
	cmd.Stderr = werr
	cmd.WaitDelay = scriptGracePeriod
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}

	defer func() {
		os.Remove(rec.LockFilePath)

		replacePart := ".partial.log"

		idx := strings.LastIndex(rec.LogFilePath, replacePart)

		finalLogName := strings.Replace(rec.LogFilePath, replacePart, ".log", idx)

		if err := os.Rename(rec.LogFilePath, finalLogName); err != nil {
			// FIXME: proper error handling
			fmt.Println("moving job log file:", err)
		} else {
			m.mu.Lock()
			tmpRec := m.jobs[rec.Status.InstanceID+"-"+jobID]
			tmpRec.LogFilePath = finalLogName
			m.jobs[jobID] = tmpRec
			m.mu.Unlock()
		}
	}()

	// TODO: get a better sync mechanism
	m.mu.Lock()
	tmpRec := m.jobs[rec.Status.InstanceID+"-"+jobID]
	tmpRec.Status.StartedAt = time.Now().Format(time.RFC3339)
	tmpRec.Status.State = JobRunning
	m.jobs[jobID] = tmpRec
	m.mu.Unlock()

	if err := m.dumpJobs(); err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	cmdExecErr := cmd.Wait()

	m.mu.Lock()
	tmpRec = m.jobs[rec.Status.InstanceID+"-"+jobID]
	if cmdExecErr != nil {
		tmpRec.Status.State = JobFailed
	} else {
		tmpRec.Status.State = JobDone
	}
	tmpRec.Status.FinishedAt = time.Now().Format(time.RFC3339)
	tmpRec.Status.ExitCode = cmd.ProcessState.ExitCode()
	m.jobs[rec.Status.InstanceID+"-"+jobID] = tmpRec
	m.mu.Unlock()

	if err := m.dumpJobs(); err != nil {
		return err
	}

	if cmdExecErr != nil {
		return cmdExecErr
	}

	return nil
}

func (m *JobManager) startExec(ctx context.Context, instanceID, lockFilePath, jobType, jobID string, cmd *exec.Cmd) error {
	logFilePath := filepath.Join(cmd.Dir, "jobs", jobID+".partial.log")

	jobCtx, cancel := context.WithCancel(ctx)
	rec := &JobRecord{
		Status: JobStatus{
			ID:         jobID,
			State:      JobPending,
			InstanceID: instanceID,
			Type:       jobType,
		},
		ctx:          jobCtx,
		cancel:       cancel,
		LockFilePath: lockFilePath,
		LogFilePath:  logFilePath,
	}

	m.mu.Lock()

	m.jobs[instanceID+"-"+jobID] = rec

	m.mu.Unlock()

	go func() {
		if err := m.run(rec, cmd); err != nil {
			fmt.Println("job failed", err)
		}
	}()

	return nil
}

var instanceNames = []string{
	"aardwolf",
	"axolotl",
	"babirusa",
	"bilby",
	"binturong",
	"bongo",
	"caracal",
	"coati",
	"dhole",
	"dikdik",
	"dugong",
	"echidna",
	"fossa",
	"gerenuk",
	"genet",
	"goura",
	"hirola",
	"indri",
	"jerboa",
	"kakapo",
	"kinkajou",
	"klipspringer",
	"kudu",
	"linsang",
	"margay",
	"markhor",
	"numbat",
	"okapi",
	"onager",
	"olingo",
	"pademelon",
	"pangolin",
	"pika",
	"quokka",
	"quoll",
	"saiga",
	"serval",
	"solenodon",
	"takin",
	"tamandua",
	"tarsier",
	"tenrec",
	"tuatara",
	"uakari",
	"vaquita",
	"wallaroo",
	"xerus",
	"yapok",
	"zorilla",
	"agouti",
	"anoa",
	"bharal",
	"cacomistle",
	"cuscus",
	"galago",
	"iriomote",
	"jentink",
	"kiang",
	"kouprey",
	"manul",
	"muntjac",
	"nyala",
	"oryx",
	"potto",
	"sifaka",
	"tahr",
	"vicuna",
	"viscacha",
	"zebu",
	"zokor",
}
