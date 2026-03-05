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

var callbackURL string

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

	http.HandleFunc("/fe/api/v1/send", handleSendSMS)
	http.HandleFunc("/health", healthHandler)

	log.Println("🧪 Mockerservice started")
	log.Println("🚀 Listening on port:", port)
	log.Println("📡 Callback URL:", callbackURL)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleSendSMS(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()

	username := query.Get("username")
	to := query.Get("to")
	from := query.Get("from")
	text := query.Get("text")
	dltContentId := query.Get("dltContentId")

	log.Println("📥 Send Request Received")
	log.Println("username:", username)
	log.Println("to:", to)
	log.Println("from:", from)
	log.Println("dltContentId:", dltContentId)

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

	// async callback
	go simulateCallback(txnID, to, from, text)
}

func simulateCallback(txnID int64, to, from, text string) {

	time.Sleep(5 * time.Second)

	log.Printf("🕒 Triggering callback for txnID=%d", txnID)

	params := url.Values{}
	params.Set("txid", fmt.Sprintf("%d", txnID))
	params.Set("to", to)
	params.Set("from", from)
	params.Set("description", "DELIVERED")
	params.Set("pdu", "1")
	params.Set("text", text)
	params.Set("deliverystatus", "DELIVERY_SUCCESS")
	params.Set("deliverydt", time.Now().Format("2006-01-02 15:04:05"))

	fullURL := callbackURL + "?" + params.Encode()

	log.Println("📡 Callback URL:", fullURL)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(fullURL)
	if err != nil {
		log.Println("❌ Callback failed:", err)
		return
	}
	defer resp.Body.Close()

	log.Println("✅ Callback completed:", resp.Status)
}
