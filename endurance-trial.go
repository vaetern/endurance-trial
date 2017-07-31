package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var cpusToWork int
var minutesToRun int

const secondsToReportFromWorker = 1
const secondsToReportWrite = 10

const timeToSleep = time.Microsecond * 500

func main() {

	fs := flag.NewFlagSet("endurance-trial", flag.ExitOnError)
	cpus := fs.Int("cpus", runtime.NumCPU(), "Number of CPUs to use")
	targets := fs.String("targets", "", "List of targets")
	minutes := fs.Int("minutes", 1, "Minutes to run trial")
	resultsFilePath := fs.String("results", "./results.txt", "List of results")

	fs.Parse(os.Args[1:])

	cpusToWork = *cpus

	minutesToRun = *minutes

	runtime.GOMAXPROCS(*cpus)

	fs.Usage = func() {
		fmt.Println("Usage: endurance-trial.exe -cpus=2 -targets=targets.txt")
	}

	if *targets == "" {
		fs.Usage()
		os.Exit(1)
	}

	targetsList, err := readFileWithReadString(targets)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	targetsDispenser := make(chan string, 1000)
	resultGathering := make(chan trialResult)

	var wg sync.WaitGroup
	wg.Add(4)

	go spinTargetsDispenser(targetsList, targetsDispenser, &wg)

	for i := 0; i < *cpus; i++ {
		go doTrialRoutine(targetsDispenser, resultGathering, &wg)
	}

	go resultSaver(resultsFilePath, resultGathering, &wg)

	wg.Wait()
}

func resultSaver(resultsFilePath *string, resultGathering chan trialResult, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println(" > Start results saver")
	fileToWriteIn, err := os.OpenFile(*resultsFilePath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Println(" ! Failed to open file for writing results: " + *resultsFilePath)
		fmt.Println(err)
	}
	defer fileToWriteIn.Close()

	var successfulHitsCount, failedHitCount uint32
	successfulHitsCount = 0
	failedHitCount = 0

	workersLeft := cpusToWork

	startTime := time.Now()
loopReadingResults:
	for soleTrialResult := range resultGathering {
		successfulHitsCount += soleTrialResult.successfulHitCount
		failedHitCount += soleTrialResult.failedHitCount
		if time.Since(startTime).Seconds() > secondsToReportWrite {

			startTime = time.Now()
			_, err = fileToWriteIn.WriteString(
				startTime.String() +
					": " +
					strconv.FormatUint(uint64(successfulHitsCount), 10) +
					"/" +
					strconv.FormatUint(uint64(failedHitCount), 10) +
					"\n")

			fmt.Println(startTime.String() + " - write result")

			if err != nil {
				fmt.Println(" > Failed to write results")
				panic(err)
			}
			successfulHitsCount = 0
			failedHitCount = 0
		}
		if soleTrialResult.lastForMe == true {
			workersLeft--
			if workersLeft == 0 {

				startTime = time.Now()
				_, err = fileToWriteIn.WriteString(
					startTime.String() +
						": " +
						strconv.FormatUint(uint64(successfulHitsCount), 10) +
						"/" +
						strconv.FormatUint(uint64(failedHitCount), 10) +
						"\n")

				fmt.Println(startTime.String() + " - write result")

				if err != nil {
					fmt.Println(" > Failed to write results")
					panic(err)
				}

				close(resultGathering)
				break loopReadingResults
			}
		}
	}

	fmt.Println(" < Close results saver")
}

type trialResult struct {
	successfulHitCount uint32
	failedHitCount     uint32
	lastForMe          bool
}

func doTrialRoutine(targetsDispenser <-chan string, resultGathering chan<- trialResult, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println(" > Start trial worker")
	var successfulHitCount, failedHitCount uint32
	failedHitCount = 0
	successfulHitCount = 0
	start := time.Now()
	for urlToTest := range targetsDispenser {
		time.Sleep(timeToSleep)
		_, err := http.Get(urlToTest)
		if err != nil {
			failedHitCount++
			continue
		}

		successfulHitCount++

		if time.Since(start).Seconds() > secondsToReportFromWorker {
			results := trialResult{successfulHitCount, failedHitCount, false}
			resultGathering <- results
			successfulHitCount = 0
			failedHitCount = 0
		}
	}

	results := trialResult{successfulHitCount, failedHitCount, true}
	resultGathering <- results

	fmt.Println(" < Close trial worker")
}

func spinTargetsDispenser(targetsList []string, targetsDispenser chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println(" > Start spinning targets dispenser")
	startTime := time.Now()
	var counter uint64
	counter = 0

infiniteLoop:
	for {
		for _, singleTarget := range targetsList {
			targetsDispenser <- singleTarget
			counter++
			if time.Since(startTime).Seconds() > float64(minutesToRun*60) {
				fmt.Println(" < Closing targets dispenser with targets " + strconv.FormatUint(counter, 10))
				close(targetsDispenser)
				break infiniteLoop
			}
		}
	}
}

func readFileWithReadString(fn *string) (listOfTargets []string, err error) {
	file, err := os.Open(*fn)
	defer file.Close()

	if err != nil {
		fmt.Printf(" > Failed!: %v\n", err)
		return nil, err
	}

	reader := bufio.NewReader(file)

	var line string
	for {
		line, err = reader.ReadString('\n')

		listOfTargets = append(listOfTargets, line)

		if err != nil {
			break
		}
	}

	if err != io.EOF {
		fmt.Printf(" > Failed!: %v\n", err)
	}

	return listOfTargets, nil
}
