package main

import (
	"encoding/json"
	"fmt"
	"github.com/saquib.mian/prun/logwriter"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
    version = "0.1"
	runfile = "prun.json"
    timeout = time.Minute * 30
)

var (
	failed    int32
	processes sync.WaitGroup
)

// Command is a representation of a program to run
type Command struct {
	Command string
	Args    []string
}

func main() {
    fmt.Printf("prun v%s\n", version)
    
	additionalArgs := os.Args[1:]

	// get commands
	var commands []Command
	file, err := os.Open(runfile)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		os.Exit(-1)
	}
	defer file.Close()
	json.NewDecoder(file).Decode(&commands)

	// run all commands and wait
	for i, command := range commands {
		command.Args = append(command.Args, additionalArgs...)
		processes.Add(1)
		go runCommand(i+1, command)
	}
	processes.Wait()

	if failed > 0 {
		fmt.Printf("error: %d command(s) failed\n", failed)
		os.Exit(int(failed))
	}

	os.Exit(0)
}

func runCommand(workerNumber int, command Command) {
    defer processes.Done()
    
	stdout := log.New(os.Stdout, fmt.Sprintf("[%d] ", workerNumber), log.Ltime)
	stderr := log.New(os.Stderr, fmt.Sprintf("[%d] ", workerNumber), log.Ltime)

	process := exec.Command(command.Command, command.Args...)
	process.Stdout = logwriter.NewLogWriter(stdout)
	process.Stderr = logwriter.NewLogWriter(stderr)

	stdout.Printf("--> '%s %s'\n", command.Command, strings.Join(command.Args, " "))

	timer := time.NewTimer(timeout)
	go func(timer *time.Timer, process *exec.Cmd) {
        for _ = range timer.C {
			err := process.Process.Signal(os.Kill)
			if err := process.Wait(); err != nil {
				if _, ok := err.(*exec.ExitError); ok {
					stderr.Print("exited with non-zero exit code")
				} else {
					stderr.Printf("error: %s\n", err.Error())
				}
			}
			stderr.Printf("error: process timed out %s", err.Error())
            incrementFailed()
            break
		}
	}(timer, process)

	if err := process.Start(); err != nil {
		stderr.Printf("error: %s\n", err.Error())
		incrementFailed()
	} else {
		if err := process.Wait(); err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				stderr.Print("exited with non-zero exit code")
			} else {
				stderr.Printf("error: %s\n", err.Error())
			}
			incrementFailed()
		}
	}
    stdout.Print("done")
}

func incrementFailed() {
	atomic.AddInt32(&failed, 1)
}
