package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/saquib.mian/prun/logwriter"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	version = "0.1"
	runfile = "prun.json"
	timeout = time.Minute * 30
)

var (
	maxconcurrency = 4
)

func init() {
	flag.IntVar(&maxconcurrency, "n", 4, "number of commands to run at a time")
	flag.Parse()
}

// Command is a representation of a program to run
type Command struct {
	Command string
	Args    []string
}

type CommandResult struct {
	Success bool
	Error   error
}

func (c *Command) String() string {
	return fmt.Sprintf("'%s %s'", c.Command, strings.Join(c.Args, " "))
}

func main() {
	fmt.Printf("prun v%s\n", version)

	additionalArgs := flag.Args()

	// get commands
	var commands []Command
	file, err := os.Open(runfile)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		os.Exit(-1)
	}
	defer file.Close()
	json.NewDecoder(file).Decode(&commands)

	input := make(chan Command)
	output := make(chan CommandResult)

	// start workers
	for i := 1; i <= maxconcurrency; i++ {
		go worker(i, input, output)
	}

	// publish all commands to run
	go func() {
		for _, cmd := range commands {
			cmd.Args = append(cmd.Args, additionalArgs...)
			input <- cmd
		}
		close(input)
	}()

	// wait for all commands to finish
	numFailed := 0
	for i := 0; i < len(commands); i++ {
		result := <-output
		if !result.Success {
			numFailed++
		}
	}

	if numFailed > 0 {
		fmt.Printf("error: %d command(s) failed\n", numFailed)
		os.Exit(numFailed)
	}

	os.Exit(0)
}

func worker(id int, input <-chan Command, output chan<- CommandResult) {
	for cmd := range input {
		stdout := log.New(os.Stdout, fmt.Sprintf("[%d] ", id), log.Ltime)
		stderr := log.New(os.Stderr, fmt.Sprintf("[%d] ", id), log.Ltime)

		stdout.Printf("--> %s\n", cmd.String())

		stdoutWriter := logwriter.NewLogWriter(stdout)
		stderrWriter := logwriter.NewLogWriter(stderr)
		result := runCommand(stdoutWriter, stderrWriter, cmd)
		stdoutWriter.Flush()
		stderrWriter.Flush()

		if !result.Success {
			stderr.Printf("error: %s\n", result.Error.Error())
		}

		output <- result
	}
}

func runCommand(stdout io.Writer, stderr io.Writer, command Command) CommandResult {
	process := exec.Command(command.Command, command.Args...)
	process.Stdout = stdout
	process.Stderr = stderr

	if err := process.Start(); err != nil {
		return CommandResult{Error: err}
	}

	timedOut := false
	timer := time.NewTimer(timeout)
	go func(timer *time.Timer, process *exec.Cmd) {
		for _ = range timer.C {
			process.Process.Signal(os.Kill)
			timedOut = true
			break
		}
	}(timer, process)

	if err := process.Wait(); err != nil {
		if timedOut {
			err = fmt.Errorf("process timed out: %s", command.String())
		} else if _, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("exited with non-zero exit code")
		}
		return CommandResult{Error: err}
	}

	return CommandResult{Success: true}
}
