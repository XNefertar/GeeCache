package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Magic   = 0x4743
	Version = 1
)

// Stats tracks benchmark statistics
type Stats struct {
	Requests uint64
	Errors   uint64
	Bytes    uint64
}

func main() {
	var (
		target      string
		concurrency int
		duration    time.Duration
		key         string
		value       string
		cmd         string
	)

	flag.StringVar(&target, "t", "127.0.0.1:9000", "Target address (host:port)")
	flag.IntVar(&concurrency, "c", 10, "Number of concurrent connections")
	flag.DurationVar(&duration, "d", 10*time.Second, "Duration of test")
	flag.StringVar(&cmd, "cmd", "get", "Command to benchmark (get/put)")
	flag.StringVar(&key, "key", "bench_key", "Key to use")
	flag.StringVar(&value, "val", "bench_value_1234567890", "Value to use (for put)")
	flag.Parse()

	fmt.Printf("Running %s benchmark against %s\n", cmd, target)
	fmt.Printf("Concurrency: %d, Duration: %v\n", concurrency, duration)

	// Pre-populate data if testing GET
	if cmd == "get" {
		fmt.Println("Pre-populating key for GET benchmark...")
		if err := sendRequest(target, "put", key, value); err != nil {
			log.Fatalf("Failed to pre-populate data: %v", err)
		}
	}

	var stats Stats
	var wg sync.WaitGroup
	start := time.Now()
	stop := make(chan struct{})

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(target, cmd, key, value, stop, &stats)
		}()
	}

	// Run for duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	elapsed := time.Since(start)

	// Report
	reqs := atomic.LoadUint64(&stats.Requests)
	errs := atomic.LoadUint64(&stats.Errors)
	bytes := atomic.LoadUint64(&stats.Bytes)
	qps := float64(reqs) / elapsed.Seconds()
	mbps := float64(bytes) / 1024 / 1024 / elapsed.Seconds()

	fmt.Println("\n--- Benchmark Results ---")
	fmt.Printf("Total Requests: %d\n", reqs)
	fmt.Printf("Total Errors:   %d\n", errs)
	fmt.Printf("Duration:       %v\n", elapsed)
	fmt.Printf("QPS:            %.2f req/s\n", qps)
	fmt.Printf("Throughput:     %.2f MB/s\n", mbps)
}

func runWorker(target, cmd, key, val string, stop chan struct{}, stats *Stats) {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("Connection failed: %v", err)
		atomic.AddUint64(&stats.Errors, 1)
		return
	}
	defer conn.Close()

	// Reuse buffer
	buf := make([]byte, 4096)
	seq := uint64(0)

	for {
		select {
		case <-stop:
			return
		default:
		}

		seq++
		reqBody := key
		if cmd == "put" {
			// Put body format: [KeyLen 4][Key][Val]
			// Note: C++ server expects raw bytes for body, but for PUT it parses manually.
			// Let's match C++ storage_server.cc:Process logic
			// if (method == "put") ... uint32_t klen; memcpy...

			// We need to construct the specific body for PUT
			klen := uint32(len(key))
			bodyLen := 4 + len(key) + len(val)
			reqBodyBytes := make([]byte, bodyLen)
			binary.LittleEndian.PutUint32(reqBodyBytes[0:4], klen)
			copy(reqBodyBytes[4:], key)
			copy(reqBodyBytes[4+len(key):], val)
			reqBody = string(reqBodyBytes)
		}

		reqData := encode(seq, cmd, reqBody)

		_, err := conn.Write(reqData)
		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			// Reconnect
			conn.Close()
			conn, _ = net.Dial("tcp", target)
			continue
		}

		// Read response
		// 1. Read Header (8 + 4 + 8 + 2 = 22 bytes minimum? No)
		// Header: Magic(2) Ver(1) Flags(1) TotalLen(4) = 8 bytes fixed
		_, err = io.ReadFull(conn, buf[:8])
		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			continue
		}

		// Parse TotalLen
		totalLen := binary.BigEndian.Uint32(buf[4:8])

		// Read remaining (Seq + MetaLen + Meta + Body)
		_, err = io.ReadFull(conn, buf[:totalLen])
		if err != nil {
			atomic.AddUint64(&stats.Errors, 1)
			continue
		}

		atomic.AddUint64(&stats.Requests, 1)
		atomic.AddUint64(&stats.Bytes, uint64(8+totalLen))
	}
}

func sendRequest(target, cmd, key, val string) error {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		return err
	}
	defer conn.Close()

	reqBody := key
	if cmd == "put" {
		klen := uint32(len(key))
		bodyLen := 4 + len(key) + len(val)
		reqBodyBytes := make([]byte, bodyLen)
		binary.LittleEndian.PutUint32(reqBodyBytes[0:4], klen)
		copy(reqBodyBytes[4:], key)
		copy(reqBodyBytes[4+len(key):], val)
		reqBody = string(reqBodyBytes)
	}

	data := encode(1, cmd, reqBody)
	_, err = conn.Write(data)
	return err
}

// encode implements the v2 protocol serialization
func encode(seq uint64, method string, body string) []byte {
	// Meta: "method" -> method
	// Meta Format: [KLen 2][Key][VLen 2][Val]
	metaBuf := make([]byte, 0, 64)

	// Key: "method"
	k := "method"
	metaBuf = append(metaBuf, 0, 0) // Placeholder for KLen
	binary.LittleEndian.PutUint16(metaBuf[len(metaBuf)-2:], uint16(len(k)))
	metaBuf = append(metaBuf, k...)

	// Val: method string
	metaBuf = append(metaBuf, 0, 0) // Placeholder for VLen
	binary.LittleEndian.PutUint16(metaBuf[len(metaBuf)-2:], uint16(len(method)))
	metaBuf = append(metaBuf, method...)

	metaLen := len(metaBuf)
	bodyLen := len(body)
	totalLen := 8 + 2 + metaLen + bodyLen // Seq(8) + MetaLen(2) + Meta + Body

	// Frame: [Magic 2][Ver 1][Flags 1][TotalLen 4] ...
	frame := make([]byte, 8+totalLen)

	// 1. Fixed Header
	binary.BigEndian.PutUint16(frame[0:2], Magic)
	frame[2] = Version
	frame[3] = 0 // Flags
	binary.BigEndian.PutUint32(frame[4:8], uint32(totalLen))

	// 2. Seq (Little Endian as per C++ memcpy)
	binary.LittleEndian.PutUint64(frame[8:16], seq)

	// 3. MetaLen (Little Endian)
	binary.LittleEndian.PutUint16(frame[16:18], uint16(metaLen))

	// 4. Meta
	copy(frame[18:], metaBuf)

	// 5. Body
	copy(frame[18+metaLen:], body)

	return frame
}
