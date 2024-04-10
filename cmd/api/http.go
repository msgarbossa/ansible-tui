package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
)

type playbookRequest struct {
	ctx           context.Context
	payload       PlaybookConfig
	outputChannel chan error
}

const maxPlaybookConcurrent = 5

// global variables
var (
	regExpHostPort = regexp.MustCompile(`(^[a-zA-Z0-9./\-_]?)\:([0-9]{2,5})$`)
	requestChannel chan playbookRequest
)

func Enqueue(ctx context.Context, request PlaybookConfig, outputChan chan error) error {
	slog.Info("Running Enqueue")

	select {
	case requestChannel <- playbookRequest{
		ctx:           ctx,
		payload:       request,
		outputChannel: outputChan,
	}:
		slog.Info("Finished putting playbookRequest on requestChannel")
		return nil

	default:
		slog.Warn("Work queue full")
		return &ExecutionError{
			Err: errors.New("work queue full"),
		}
	}
}

func handle(
	ctx context.Context,
	request PlaybookConfig,
	outputChan chan error,
) {

	slog.Info("Running handle")
	// recover from panic
	defer func() {
		if err := recover(); err != nil {
			slog.Error(fmt.Sprintf("%s", err))
			// notify ouput channel
			outputChan <- err.(error)
		} else {
			slog.Info("putting nil on outputChan in recover()")
			outputChan <- nil
		}
	}()

	request.validateInputs()

	rc, err := request.runAnsiblePlaybook()
	if err != nil {
		slog.Error("Error running playbook: %s", err)
	}

	request.Metrics.ExitCode = rc
	request.Metrics.Error = err

	// time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

	slog.Info("Handle finished")
	slog.Info("putting nil on outputChan")
	outputChan <- nil
	<-ctx.Done()
}

func process(worker int) {
	for c := range requestChannel {
		slog.Info(fmt.Sprintf("Worker %d processing request from channel", worker))
		handle(c.ctx, c.payload, c.outputChannel)
	}
}

func mainListener() {
	var (
		ctx, cancel = context.WithCancel(context.Background())
	)

	defer func() {
		fmt.Println("Running cancel()")
		cancel()
	}()

	if !regExpHostPort.MatchString(httpAddr) {
		slog.Error("httpAddr does not match expected hostname:port pattern")
		return
	}

	requestChannel = make(chan playbookRequest, maxPlaybookConcurrent)

	defer func() {
		close(requestChannel)
	}()

	// spawn workers
	for i := 0; i < maxPlaybookConcurrent; i++ {
		slog.Info("Starting listener")
		go process(i)
	}

	go startHttpListener(ctx)

	// Wait for SIGINT.
	sig := make(chan os.Signal, 3)
	signal.Notify(sig, syscall.SIGHUP)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

}

func startHttpListener(ctx context.Context) {

	defer func() {
		log.Println("running deferred ctx.Done in startHttpListener")
		select {
		case <-ctx.Done():
			log.Println("startHttpListener cancelled due to error: ", ctx.Err())
			return
		default:
			log.Println("startHttpListener was not cancelled")
			return
		}
	}()

	http.HandleFunc("/ansible", handleAnsible)
	slog.Info("server started: " + httpAddr)
	log.Fatal(http.ListenAndServe(httpAddr, nil))

}

func handleAnsible(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed) // 405
		return
	}

	slog.Info(fmt.Sprintf("headers: %v\n", r.Header))

	bodyBytes, err := readBodyBytes((r))
	if err != nil {
		slog.Error(fmt.Sprintf("Body read error: %s", err))
		w.WriteHeader(500) // Return 500 Internal Server Error.
		return
	}

	slog.Debug(fmt.Sprintf("body: %s\n", bodyBytes))

	pb := PlaybookConfig{}
	if err := json.Unmarshal(bodyBytes, &pb); err != nil {
		log.Fatal(err)
		slog.Error(fmt.Sprintf("Body parse error: %s", err))
		w.WriteHeader(http.StatusInternalServerError) // 500
		return
	}

	outputChan := make(chan error)

	err = Enqueue(
		context.TODO(),
		pb,
		outputChan,
	)

	// err = <-outputChan
	// if err != nil {
	// 	slog.Warn("Error on outputChan, send http.StatusInternalServerError")
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	return
	// }

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError) // 500
	} else {
		w.WriteHeader(http.StatusOK) // 200 OK
	}

	slog.Info("Finished processing ansible event")

}

func readBodyBytes(r *http.Request) ([]byte, error) {
	// Read body
	bodyBytes, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return nil, readErr
	}
	defer r.Body.Close()

	// GZIP decode
	if len(r.Header["Content-Encoding"]) > 0 && r.Header["Content-Encoding"][0] == "gzip" {
		contents, gzErr := gzip.NewReader(io.NopCloser(bytes.NewBuffer(bodyBytes)))
		if gzErr != nil {
			return nil, gzErr
		}
		defer contents.Close()

		bb, err2 := io.ReadAll(contents)
		if err2 != nil {
			return nil, err2
		}
		return bb, nil
	} else {
		// Not compressed
		return bodyBytes, nil
	}
}
