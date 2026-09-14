package main

import (
	"bytes"
	"context"
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
	status       JobStatus
	ctx          context.Context
	cancel       context.CancelFunc
	lockFilePath string
	logFilePath  string
}

type JobManager struct {
	path string
	sem  chan struct{}

	mu sync.RWMutex

	jobs map[string]*JobRecord

	wg sync.WaitGroup
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

func (m *JobManager) run(ctx context.Context, rec *JobRecord, cmd *exec.Cmd) error {
	f, err := os.OpenFile(rec.logFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
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
		os.Remove(rec.lockFilePath)

		replacePart := ".partial.log"

		idx := strings.LastIndex(rec.logFilePath, replacePart)

		newLogName := strings.Replace(rec.logFilePath, replacePart, ".log", idx)

		if err := os.Rename(rec.logFilePath, newLogName); err != nil {
			// FIXME: proper error handling
			fmt.Println("moving job log file:", err)
		}
	}()

	rec.status.StartedAt = time.Now().Format(time.RFC3339)
	rec.status.State = JobRunning

	if err := cmd.Start(); err != nil {
		return err
	}

	batcher := NewLogBatcher(256)

	go func() {
		err := batcher.run(ctx, 100*time.Millisecond, func(batch LogBatch) error {
			// TODO: figure out how do we notify client
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Println("log batcher:", err)
		}
	}()

	// TODO: process stdout and push notifications through jsonrpc

	defer batcher.Complete(ctx)

	err = cmd.Wait()

	rec.status.State = JobDone
	rec.status.FinishedAt = time.Now().Format(time.RFC3339)
	rec.status.ExitCode = cmd.ProcessState.ExitCode()

	if err != nil {
		rec.status.State = JobFailed
		return err
	}

	return nil
}

func (m *JobManager) startExec(ctx context.Context, instanceID, lockFilePath, jobID string, cmd *exec.Cmd) error {
	logFilePath := filepath.Join(cmd.Dir, "jobs", jobID+".partial.log")

	jobCtx, cancel := context.WithCancel(ctx)
	rec := &JobRecord{
		status: JobStatus{
			State:      JobPending,
			InstanceID: instanceID,
		},
		ctx:          jobCtx,
		cancel:       cancel,
		lockFilePath: lockFilePath,
		logFilePath:  logFilePath,
	}

	m.mu.Lock()

	m.jobs[jobID] = rec

	m.mu.Unlock()

	go func() {
		if err := m.run(jobCtx, rec, cmd); err != nil {
			fmt.Println("job failed", err)
		}
	}()

	return nil
}

func (m *JobManager) log(instancePath string, jobName string) {
	partialPath := filepath.Join(instancePath, "jobs", jobName+".partial.log")

	f, err := os.OpenFile(partialPath, os.O_RDONLY, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			fullPath := filepath.Join(instancePath, "jobs", jobName+".log")

			f, err := os.OpenFile(fullPath, os.O_RDONLY, 0o644)
			if err != nil {
				return
			}

			defer f.Close()
		}
		return
	}
	defer f.Close()
}

type LogBatch struct {
	Data  []byte
	Final bool
}

type batchEvent struct {
	data  []byte
	final bool
}

type LogBatcher struct {
	events chan batchEvent
}

func NewLogBatcher(bufferSize int) *LogBatcher {
	return &LogBatcher{
		events: make(chan batchEvent, bufferSize),
	}
}

func (b *LogBatcher) Publish(ctx context.Context, data []byte) error {
	data = bytes.Clone(data)

	select {

	case b.events <- batchEvent{data: data}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *LogBatcher) Complete(ctx context.Context) error {
	select {
	case b.events <- batchEvent{final: true}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *LogBatcher) run(ctx context.Context, interval time.Duration, send func(LogBatch) error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var pending bytes.Buffer

	flush := func(final bool) error {
		if pending.Len() == 0 && !final {
			return nil
		}

		data := bytes.Clone(pending.Bytes())
		pending.Reset()
		return send(LogBatch{
			Data:  data,
			Final: final,
		})
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event := <-b.events:
			if len(event.data) > 0 {
				_, _ = pending.Write(event.data)
			}

			if event.final {
				return flush(true)
			}

		case <-ticker.C:
			if err := flush(false); err != nil {
				return err
			}
		}
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
