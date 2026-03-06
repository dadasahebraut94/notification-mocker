package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type SmartPingAPIResponse struct {
	TransactionID int64  `json:"transactionId"`
	State         string `json:"state"`
	StatusCode    int    `json:"statusCode"`
	Description   string `json:"description"`
	PDU           int    `json:"pdu"`
}

type CallbackJob struct {
	TxnID int64
	To    string
	From  string
	Text  string
}

var (
	callbackURL    string
	jobQueue       = make(chan CallbackJob, 1_000_000) // All incoming jobs
	readyQueue     = make(chan CallbackJob, 10_000)    // Jobs ready to be sent after jitter
	numSleepers    = 20_000                            // High number of goroutines for parallel sleep
	numHTTPWorkers = 1000                              // Controlled number of concurrent HTTP connections
	httpClient     = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 500,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

func main() {
	// load .env file (for local development)
	godotenv.Load()

	rand.Seed(time.Now().UnixNano())

	// Render provides PORT automatically
	port := os.Getenv("PORT")
	if port == "" {
		port = "5003"
	}

	// Callback URL from environment
	callbackURL = os.Getenv("CALLBACK_URL")
	if callbackURL == "" {
		callbackURL = "http://localhost:5002/api/v1/o/sms/status/smart-ping"
	}

	// 1. Start HTTP Workers (The ones doing the real network work)
	for i := 1; i <= numHTTPWorkers; i++ {
		go httpWorker(i)
	}

	// 2. Start Sleeper Workers (The ones handling the jitter delay)
	for i := 1; i <= numSleepers; i++ {
		go sleeperWorker(i)
	}

	http.HandleFunc("/fe/api/v1/send", handleSendSMS)
	http.HandleFunc("/health", healthHandler)

	log.Printf("🧪 Mockerservice started with %d sleepers and %d http workers\n", numSleepers, numHTTPWorkers)
	log.Println("🚀 Listening on port:", port)
	log.Println("📡 Callback URL:", callbackURL)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// sleeperWorker waits for jitter without blocking the HTTP workers
func sleeperWorker(id int) {
	for job := range jobQueue {
		// Random jitter range: 0 to 300 seconds
		jitter := time.Duration(rand.Intn(300)) * time.Second
		time.Sleep(jitter)

		// Move job to readyQueue after sleep
		select {
		case readyQueue <- job:
			// Job is now ready for HTTP worker
		default:
			// If readyQueue is full, this sleeper blocks, effectively providing backpressure
			readyQueue <- job
		}
	}
}

// httpWorker handles the actual network communication
func httpWorker(id int) {
	for job := range readyQueue {
		processCallback(job)
	}
}

func handleSendSMS(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	to := query.Get("to")
	from := query.Get("from")
	text := query.Get("text")

	// generate random transaction ID
	txnID := rand.Int63n(9000000000) + 1000000000

	resp := SmartPingAPIResponse{
		TransactionID: txnID,
		State:         "SUBMIT_ACCEPTED",
		StatusCode:    200,
		Description:   "Request accepted",
		PDU:           1,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	// Queue the job for jitter processing (Non-blocking)
	job := CallbackJob{
		TxnID: txnID,
		To:    to,
		From:  from,
		Text:  text,
	}

	select {
	case jobQueue <- job:
		// Queued successfully
	default:
		log.Printf("⚠️ Global Queue full, dropping job for txnID=%d", txnID)
	}
}

func processCallback(job CallbackJob) {
	params := url.Values{}
	params.Set("txid", fmt.Sprintf("%d", job.TxnID))
	params.Set("to", job.To)
	params.Set("from", job.From)
	params.Set("text", job.Text)
	params.Set("pdu", "1")
	params.Set("deliverydt", time.Now().Format("2006-01-02 15:04:05"))

	params.Set("description", "UNDELIVERED")
	params.Set("deliverystatus", "DELIVERY_FAILURE")

	fullURL := callbackURL + "?" + params.Encode()

	resp, err := httpClient.Get(fullURL)
	if err != nil {
		log.Printf("❌ Callback failed for txnID=%d: %v", job.TxnID, err)
		return
	}
	defer resp.Body.Close()

	// Only log periodically to avoid flooding logs with 8 lakh entries
	if job.TxnID%100 == 0 {
		log.Printf("✅ Callback Sample (txnID=%d): %s", job.TxnID, resp.Status)
	}
}
