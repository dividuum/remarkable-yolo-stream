package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func gz_wrap(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			fn(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz, _ := gzip.NewWriterLevel(w, 1)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		fn(gzr, r)
	}
}

type Monitor struct {
	Notifier       chan []byte
	newClients     chan chan []byte
	closingClients chan chan []byte
	clients        map[chan []byte]bool
}

func NewMonitor() (monitor *Monitor) {
	monitor = &Monitor{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}
	go monitor.listen()
	return
}

func (monitor *Monitor) sse(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	flusher.Flush()

	messageChan := make(chan []byte)
	monitor.newClients <- messageChan

	defer func() {
		monitor.closingClients <- messageChan
	}()

	notify := r.Context().Done()

	go func() {
		<-notify
		monitor.closingClients <- messageChan
	}()

	for {
		fmt.Fprintf(w, "data: %s\n\n", <-messageChan)
		flusher.Flush()
	}
}

func (monitor *Monitor) listen() {
	for {
		select {
		case s := <-monitor.newClients:
			monitor.clients[s] = true
		case s := <-monitor.closingClients:
			delete(monitor.clients, s)
		case event := <-monitor.Notifier:
			for clientMessageChan, _ := range monitor.clients {
				clientMessageChan <- event
			}
		}
	}
}

func (monitor *Monitor) device(device string) {
	dev, err := os.OpenFile(device, os.O_RDONLY, 0644)
	if err != nil {
		panic(err)
	}
	r := bufio.NewReader(dev)
	for {
		ev := make([]byte, 16) // sizeof(struct input_event)
		_, err := io.ReadFull(r, ev)
		if err != nil {
			panic(err)
		}
		monitor.Notifier <- []byte("x")
	}
}

var (
	frameAddr  int64
	xochitlMem io.ReaderAt
	monitor    *Monitor

	//go:embed index.html
	index []byte

	//go:embed stream.js
	js []byte
)

func getPid() string {
	content, err := ioutil.ReadFile("/tmp/epframebuffer.lock")
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(content), "\n")
	return lines[0]
}

func getFrameAddr(pid string) int64 {
	file, err := os.Open("/proc/" + pid + "/maps")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// XXX: why the end mapped address space!?
	re := regexp.MustCompile("[0-9a-f]+-([0-9a-f]+) .* /dev/fb0")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); m != nil {
			addr, err := strconv.ParseInt("0x"+m[1], 0, 64)
			if err != nil {
				panic(err)
			}
			return addr
		}
	}
	panic("cannot find pattern")
	return 0
}

func handler(w http.ResponseWriter, r *http.Request) {
	raw := make([]uint8, 1872*1404*2)
	n, err := xochitlMem.ReadAt(raw, frameAddr)
	if err != nil || n != len(raw) {
		panic(err)
	}
	frame := make([]uint8, len(raw)/2)
	for i := 0; i < len(frame); i++ {
		frame[i] = raw[i*2+1]
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(frame)
}

func getMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/raw", gz_wrap(handler))
	mux.HandleFunc("/events", monitor.sse)
	mux.HandleFunc("/stream.js", func(w http.ResponseWriter, _ *http.Request) {
		io.Copy(w, bytes.NewReader(js))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.Copy(w, bytes.NewReader(index))
	})
	return mux
}

func main() {
	monitor = NewMonitor()
	pid := getPid()
	xochitlMem, _ = os.OpenFile("/proc/"+pid+"/mem", os.O_RDONLY, os.ModeDevice)
	frameAddr = getFrameAddr(pid) + 5259272 // XXX: what?
	go monitor.device("/dev/input/event1")
	go monitor.device("/dev/input/event2")
	http.ListenAndServe(":1113", getMux())
}
