package stream

import (
	"bufio"
	"fmt"
	"github.com/diggs/go-backoff"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	// STREAM_INACTIVITY_TIMEOUT_SECONDS specifies the amount of time to wait between receiving data
	// before a stall condition is detected and the connection is backed off.
	STREAM_INACTIVITY_TIMEOUT_SECONDS int = 4
)

type HttpStream struct {
	// HttpClient can be set to provide a custom HTTP client, useful if URL serves a self-signed SSL cert and validation errors need to be ignored, for example.
	HttpClient *http.Client
	// HttpRequest can be set to provide a custom HTTP request, useful in cases where the default HTTP GET verb is not appropriate, for example.
	HttpRequest *http.Request
	// URL specifies the endpoint to connect to - this will be ignored if a custom HttpRequest is set.
	Url string
	// Headers to send with the request when connecting to URL - this will be ignored if a custom HttpRequest is set.
	Headers map[string]string
	// Data provides the data channel that is handed each data chunk that is read from the stream.
	Data chan []byte
	// Error can be read to be notified of any connection errors that occur during the lifetime of the stream.
	// Fatal errors will be delivered on this channel before the stream is closed permanently via Close().
	// Reading from this channel is optional, it will not block if there is no reader.
	Error chan error
	// Exit can be read to be notified when the stream has exited permanently e.g. due to Close() being called, or a fatal error occurring.
	// Reading from this channel is optional, it will not block if there is no reader.
	Exit                chan bool
	exiting             bool
	waitGroup           *sync.WaitGroup
	tcpBackoff          *backoff.Backoff
	httpBackoff         *backoff.Backoff
	httpThrottleBackoff *backoff.Backoff
}

// Connect to the configured URL and begin reading data.
func (s *HttpStream) Connect() {
	go s.enterReadStreamLoop()
}

// Close permanently disconnects the stream reader and cleans up all resources.
func (s *HttpStream) Close() {
	if s.exiting {
		return
	}
	s.exiting = true
	close(s.Exit)
	go func() {
		s.waitGroup.Wait()
		close(s.Data)
		close(s.Error)
	}()
}

func (s *HttpStream) resetBackoffs() {
	s.tcpBackoff.Reset()
	s.httpBackoff.Reset()
	s.httpThrottleBackoff.Reset()
}

func (s *HttpStream) sendErr(err error) {
	// write to error chan without blocking if there are no readers
	select {
	case s.Error <- err:
	default:
	}
}

func (s *HttpStream) connect() (*http.Response, error) {

	log.Printf("Establishing connection to %s...", s.Url)

	req := s.HttpRequest
	if req == nil {
		var err error
		req, err = http.NewRequest("GET", s.Url, nil)
		if err != nil {
			return nil, err
		}

		for key, val := range s.Headers {
			req.Header.Set(key, val)
		}
	}

	resp, err := s.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *HttpStream) connectAndReadStream() {
	resp, err := s.connect()
	if err != nil {
		// TODO Differentiate between transient tcp/ip errors and fatal errors (such as malformed url etc.)
		// and close the stream if appropriate.
		s.sendErr(err)
		//log.Printf("Encountered error establishing connection: %v", err)
		//log.Printf("Backing off %d milliseconds", s.tcpBackoff.NextDuration/time.Millisecond)
		s.tcpBackoff.Backoff()
		return
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		log.Printf("Connection established...")
		s.resetBackoffs()
		s.enterReadDataLoop(resp.Body)
	case 420:
		//log.Printf("Encountered 420 backoff code")
		//log.Printf("Backing off %d minute(s)", s.httpThrottleBackoff.NextDuration/time.Minute)
		s.httpThrottleBackoff.Backoff()
	case 401:
		err = fmt.Errorf("Encountered fatal status code: %v", resp.StatusCode)
		log.Printf(err.Error())
		log.Printf("Fatal error; Closing stream.")
		s.sendErr(err)
		s.Close()
	default:
		err = fmt.Errorf("Encountered resumable status code: %v", resp.StatusCode)
		s.sendErr(err)
		log.Printf(err.Error())
		log.Printf("Backing off %d second(s)", s.httpBackoff.NextDuration/time.Second)
		s.httpBackoff.Backoff()
	}
}

func (s *HttpStream) enterReadStreamLoop() {
	s.waitGroup.Add(1)
	defer s.waitGroup.Done()

	log.Printf("Entering read stream loop...")
	for {
		select {
		case <-s.Exit:
			log.Printf("Exit signalled; leaving read stream loop.")
			return
		default:
			s.connectAndReadStream()
		}
	}
}

func (s *HttpStream) enterReadDataLoop(reader io.Reader) {

	log.Printf("Entering read data loop...")

	scanner := bufio.NewScanner(reader)
	for {
		dataCh, errCh := s.readData(scanner)
		select {
		case data := <-dataCh:
			//log.Printf("Read data chunk from stream: %d bytes.", len(data))
			if len(data) > 0 { // drop empty heartbeats
				s.Data <- data
			}
		case <-s.Exit:
			log.Printf("Exit signalled; leaving read data loop.")
			return
		case err := <-errCh:
			log.Printf("Stream error; leaving read data loop: %v", err)
			s.sendErr(err)
			return
		case <-time.After(time.Duration(STREAM_INACTIVITY_TIMEOUT_SECONDS) * time.Second):
			log.Printf("Stream inactive for %d seconds; leaving read data loop.", STREAM_INACTIVITY_TIMEOUT_SECONDS)
			return
		}
	}
}

func (s *HttpStream) readData(scanner *bufio.Scanner) (<-chan []byte, <-chan error) {
	//log.Printf("Scanning for data...")
	dataCh := make(chan []byte)
	errCh := make(chan error)
	go func() {
		if ok := scanner.Scan(); !ok {
			errCh <- scanner.Err()
			return
		}
		dataCh <- scanner.Bytes()[:]
	}()
	return dataCh, errCh
}

// NewStream creates a new stream instance.
// Override any desired properties of the httpStream object before calling Connect() to begin reading data.
func NewStream(url string) *HttpStream {
	s := HttpStream{}
	s.HttpClient = &http.Client{}
	s.HttpRequest = nil
	s.Url = url
	s.Data = make(chan []byte)
	s.Error = make(chan error)
	s.Exit = make(chan bool)
	s.waitGroup = &sync.WaitGroup{}

	s.tcpBackoff = backoff.NewLinear(250*time.Millisecond, 1000*time.Millisecond)
	// Back off exponentially, starting at 5 seconds, capping at 320 seconds
	s.httpBackoff = backoff.NewExponential(1*time.Second, 5*time.Second)
	// Back off exponentially, starting at 1 minute, with no cap
	s.httpThrottleBackoff = backoff.NewExponential(1*time.Second, 5*time.Second)
	return &s
}
