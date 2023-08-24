package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"io/ioutil"
	"log"
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

func makeGzipHandler(fn http.HandlerFunc) http.HandlerFunc {
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

var (
	pointerAddr int64
	file        io.ReaderAt

	//go:embed index.html
	index []byte

	//go:embed stream.js
	js []byte
)

func getPid() string {
	content, err := ioutil.ReadFile("/tmp/epframebuffer.lock")
	if err != nil {
		log.Fatal("Error reading the file:", err)
	}
	lines := strings.Split(string(content), "\n")
	return lines[0]
}

func getOffset(pid string) int64 {
	filePath := "/proc/" + pid + "/maps"
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Error opening file:", err)
	}
	defer file.Close()

	// XXX: why the end mapped address space!?
	re := regexp.MustCompile("[0-9a-f]+-([0-9a-f]+) .* /dev/fb0")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); m != nil {
			addr, err := strconv.ParseInt("0x"+m[1], 0, 64)
			if err != nil {
				log.Fatalf("Fail:", err)
			}
			return addr
		}
	}
	log.Fatal("cannot find pattern")
	return 0
}

func handler(w http.ResponseWriter, r *http.Request) {
	data := make([]uint8, 1872*1404*2)
	n, err := file.ReadAt(data, pointerAddr)
	if err != nil || n != len(data) {
		log.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func getMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/raw", makeGzipHandler(handler))
	mux.HandleFunc("/stream.js", func(w http.ResponseWriter, _ *http.Request) {
		io.Copy(w, bytes.NewReader(js))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.Copy(w, bytes.NewReader(index))
	})
	return mux
}

func main() {
	pid := getPid()
	file, _ = os.OpenFile("/proc/"+pid+"/mem", os.O_RDONLY, os.ModeDevice)
	pointerAddr = getOffset(pid) + 5259272 // XXX: what?
	http.ListenAndServe(":1113", getMux())
}
