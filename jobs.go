package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// this is a file for handling jobs: whatever client requested us to do and we essentially run it in async mode

type JobID string

type JobRecord struct {
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

	defer os.Remove(rec.lockFilePath)

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (m *JobManager) startExec(ctx context.Context, lockFilePath, jobName string, cmd *exec.Cmd) error {
	logFilePath := filepath.Join(cmd.Dir, "jobs", jobName+".partial.log")

	jobCtx, cancel := context.WithCancel(ctx)
	rec := &JobRecord{
		ctx:          jobCtx,
		cancel:       cancel,
		lockFilePath: lockFilePath,
		logFilePath:  logFilePath,
	}

	go func() {
		if err := m.run(jobCtx, rec, cmd); err != nil {
			fmt.Println("job failed", err)
		}
	}()

	return nil
}

func (m *JobManager) log(instancePath string, jobName string) {
	partialPath := filepath.Join(instancePath, "jobs", jobName+".partial.log")

	_, err := os.OpenFile(partialPath, os.O_RDONLY, 0o644)
	if err != nil {
		return
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
