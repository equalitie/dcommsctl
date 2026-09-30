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
	"sync"
	"sync/atomic"
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
	ctx context.Context

	mu     sync.RWMutex
	dumpMu sync.RWMutex
	jobs   map[string]*JobRecord

	jobsDBpath string

	jobsPath string
}

// `LogChunk` is the raw log data
// `NextOffset` is the cursor to pass to the next read.
// `Finished` reports that the log is COMPLETE (reaching the end doesn't mean the job is done).
type JobLogChunk struct {
	LogChunk   []byte
	NextOffset int64
	Finished   bool
}

const maxLogChunk = 64 * 1024

var jobCounter atomic.Uint64

func jobID(jobType string) string {
	return fmt.Sprintf(
		"%s-%s-%d",
		jobType,
		time.Now().UTC().Format("01-02.15:04:05.000000"),
		jobCounter.Add(1),
	)
}

func isJobActive(s JobState) bool {
	return s == JobPending || s == JobRunning
}

// jobKey is how a job is indexed in JobManager.jobs. Every read and write must
// go through it: indexing the same job under two different keys leaves the map
// holding it twice, which job/list then reports as duplicates.
func jobKey(instanceID, jobID string) string {
	return instanceID + "-" + jobID
}

func jobLockFile(lockPath string) *os.File {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		// Expected when another job of the same type is already running: the lock file
		// exists, so O_EXCL fails. The caller treats a nil file as "already locked".
		logError("cannot take job lock", "lock", lockPath, "err", err)
		return nil
	}

	return f
}

func NewJobManager(ctx context.Context) *JobManager {
	return &JobManager{
		jobs: make(map[string]*JobRecord),
		ctx:  ctx,
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

// Resolves which instance a job belongs to from its bare id.
func (m *JobManager) instanceIDForJob(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rec := range m.jobs {
		if rec.Status.ID == id {
			return rec.Status.InstanceID, true
		}
	}

	return "", false
}

func (m *JobManager) fetchLog(id string, offset int64) (JobLogChunk, error) {
	logPath, state, found := m.logSource(id)
	if !found {
		return JobLogChunk{}, fmt.Errorf("unknown job: %s", id)
	}

	f, err := os.OpenFile(logPath, os.O_RDONLY, 0o644)
	if err != nil {
		return JobLogChunk{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return JobLogChunk{}, err
	}

	size := stat.Size()

	// A cursor at or past the end of the file is not an error: it is what a client sends after
	// reading everything, and also what it sends against a job that has not printed anything yet.
	// Answering with an empty chunk is what lets Finished report the end of a log, because a
	// caller cannot tell an empty answer from a failure and would otherwise have to treat an error
	// as the end.
	if offset >= size {
		return JobLogChunk{
			NextOffset: offset,
			Finished:   !isJobActive(state),
		}, nil
	}

	length := int64(maxLogChunk)

	if offset+length > size {
		length = size - offset
	}

	buf := make([]byte, length)

	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return JobLogChunk{}, err
	}

	next := offset + int64(n)

	// For a job to be finished:
	// * The job must no longer be active
	// * AND there is no more output that has not been read.
	return JobLogChunk{
		LogChunk:   buf[:n],
		NextOffset: next,
		Finished:   next >= size && !isJobActive(state),
	}, nil
}

func (m *JobManager) logSource(id string) (logPath string, state JobState, found bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rec := range m.jobs {
		if rec.Status.ID == id {
			return rec.LogFilePath, rec.Status.State, true
		}
	}

	return "", "", false
}

func (m *JobManager) update(instanceID, id string, fn func(rec *JobRecord)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, found := m.jobs[jobKey(instanceID, id)]
	if !found {
		return false
	}

	fn(rec)

	return true
}

func (m *JobManager) startExec(ctx context.Context, instanceID, lockFilePath, jobType, jobID string, cmd *exec.Cmd) error {
	logFilePath := filepath.Join(cmd.Dir, "jobs", jobID+".log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	wout := io.MultiWriter(logFile, os.Stdout)
	werr := io.MultiWriter(logFile, os.Stderr)

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

	if err := cmd.Start(); err != nil {
		logFile.Close()
		os.Remove(logFilePath)
		os.Remove(lockFilePath)
		return err
	}

	rec := &JobRecord{
		Status: JobStatus{
			ID:         jobID,
			State:      JobRunning,
			InstanceID: instanceID,
			Type:       jobType,
			StartedAt:  time.Now().Format(time.RFC3339),
		},
		LockFilePath: lockFilePath,
		LogFilePath:  logFilePath,
	}

	m.mu.Lock()
	m.jobs[jobKey(instanceID, jobID)] = rec
	m.mu.Unlock()

	if err := m.dumpJobs(); err != nil {
		logFile.Close()
		os.Remove(lockFilePath)
		return err
	}

	go func() {
		defer func() {
			logFile.Close()
			os.Remove(lockFilePath)
		}()

		cmdExecErr := cmd.Wait()

		m.update(rec.Status.InstanceID, jobID, func(rec *JobRecord) {
			if cmdExecErr != nil {
				rec.Status.State = JobFailed
			} else {
				rec.Status.State = JobDone
			}
			rec.Status.FinishedAt = time.Now().Format(time.RFC3339)
			rec.Status.ExitCode = cmd.ProcessState.ExitCode()
		})

		if err := m.dumpJobs(); err != nil {
			logError("cannot persist job state", "err", err)
		}
	}()

	return nil
}

// Job state is persisted as a single JSON file: a map from key(instanceID, id) to a record. It
// exists so that job/status is answerable after a daemon restart, which is also why loading it is
// not a plain unmarshal -- a daemon that died mid-job leaves records that claim work is still in
// progress, and those have to be reconciled before the file is believed.
//
// The paths inside a record are stored relative to pathsRoot rather than absolutely, so a state
// file survives its tree being moved or copied. absPath and relPath are the two directions of that
// conversion.

// Load restores job state from disk, repairing anything a previous daemon left unfinished.
//
// Call this once at startup, before serving. It must happen before any job is started, because
// repairInterruptedJob's correctness depends on no job having been spawned yet in this process.
func (m *JobManager) Load() error {
	data, err := os.ReadFile(m.jobsDBpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// First run: no state file yet. Not an error.
			return nil
		}
		return err
	}

	// Keyed by the same "instanceID-jobID" string that key builds.
	var tmp map[string]*JobRecord

	if err := json.Unmarshal(data, &tmp); err != nil {
		return fmt.Errorf("jobs state unmarshal error: %w", err)
	}

	for k, rec := range tmp {
		rec.LockFilePath = m.absPath(rec.LockFilePath)
		rec.LogFilePath = m.absPath(rec.LogFilePath)

		repairInterruptedJob(k, rec)
	}

	m.mu.Lock()
	m.jobs = tmp
	m.mu.Unlock()

	// Persist, so the file stops claiming a dead job is active. MUST come after the assignment
	// above: dump marshals m.jobs, so dumping first would write an empty map over the whole file.
	// A failure is not fatal -- the repaired state is already live in memory and the repair is
	// idempotent, so the next restart redoes it.
	if err := m.dumpJobs(); err != nil {
		logError("cannot persist job state after loading it", "err", err)
	}

	return nil
}

// dump writes job state to disk. See dumpMu for why this does not hold `mu` across the write.
func (m *JobManager) dumpJobs() error {
	m.mu.Lock()

	// Written relative to pathsRoot, so a copy of the tree (or of the state file alone) still
	// resolves. A path outside pathsRoot cannot be made relative and is kept absolute, which is
	// the honest representation of a log that lives somewhere this root does not describe.
	relative := make(map[string]*JobRecord, len(m.jobs))
	for k, rec := range m.jobs {
		recCopy := *rec
		recCopy.LockFilePath = m.relPath(rec.LockFilePath)
		recCopy.LogFilePath = m.relPath(rec.LogFilePath)
		relative[k] = &recCopy
	}

	d, err := json.MarshalIndent(relative, "", "  ")

	m.mu.Unlock()

	if err != nil {
		return err
	}

	// Written outside `mu` (see dumpMu) but never concurrently with another write.
	m.dumpMu.Lock()
	defer m.dumpMu.Unlock()

	return os.WriteFile(m.jobsDBpath, d, 0o644)
}

func (m *JobManager) absPath(path string) string {
	return filepath.Join(m.jobsPath, path)
}

func (m *JobManager) relPath(path string) string {
	rel, err := filepath.Rel(m.jobsPath, path)
	if err != nil {
		return path
	}

	return rel
}

// repairInterruptedJob corrects a record left active by a daemon that stopped
// before the job could finish.
//
// This runs only from loadJobs, which only runs at startup.
//
// Note: this function is only guessing. a better way to handle this in the
// future would be to have a tiny supervisor process that runs independently
// of dcommsctl and have the supervisor record the job output and exit status.
func repairInterruptedJob(jobKey string, rec *JobRecord) {
	reason := ""

	switch rec.Status.State {
	case JobPending:
		// Never got as far as a process unless its log exists.
		if _, err := os.Stat(rec.LogFilePath); err != nil {
			return
		}

		reason = "daemon restarted while this job was starting; its outcome is unknown"

	case JobRunning:
		reason = "daemon restarted while this job was running; it was interrupted and its outcome is unknown"

	default:
		// Already terminal: an outcome was recorded and must not be overwritten.
		return
	}

	// if !rec.markInterrupted(reason) {
	// 	return
	// }

	// `reason` rather than Status.Error: markInterrupted overwrote the error, so the pre-repair
	// state is not kept and the reason is the whole explanation.
	logWarn("repaired a job left active by a previous daemon",
		"job", jobKey, "reason", reason, "log", rec.LogFilePath)

	if err := os.Remove(rec.LockFilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		logError("cannot remove lock file of interrupted job",
			"job", jobKey, "lock", rec.LockFilePath, "err", err)
	}
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
