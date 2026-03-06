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
	callbackURL   string
	callbackQueue = make(chan CallbackJob, 1000000) // Buffer for 1M jobs
	numWorkers    = 500                             // Number of concurrent workers
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

	// Start worker pool
	for i := 1; i <= numWorkers; i++ {
		go worker(i)
	}

	http.HandleFunc("/fe/api/v1/send", handleSendSMS)
	http.HandleFunc("/health", healthHandler)

	log.Printf("🧪 Mockerservice started with %d workers\n", numWorkers)
	log.Println("🚀 Listening on port:", port)
	log.Println("📡 Callback URL:", callbackURL)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func worker(id int) {
	for job := range callbackQueue {
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

	// Queue the callback job
	callbackQueue <- CallbackJob{
		TxnID: txnID,
		To:    to,
		From:  from,
		Text:  text,
	}
}

func processCallback(job CallbackJob) {
	// Add random jitter to spread the load (0 to 300 seconds as requested)
	jitter := time.Duration(rand.Intn(300)) * time.Second
	time.Sleep(jitter)

	log.Printf("🕒 Triggering callback for txnID=%d (jitter: %v)", job.TxnID, jitter)

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

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(fullURL)
	if err != nil {
		log.Printf("❌ Callback failed for txnID=%d: %v", job.TxnID, err)
		return
	}
	defer resp.Body.Close()

	log.Printf("✅ Callback completed for txnID=%d: %s", job.TxnID, resp.Status)
}
