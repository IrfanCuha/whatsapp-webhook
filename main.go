package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
)

var (
	webhookVerifyToken = os.Getenv("WEBHOOK_VERIFY_TOKEN")
	graphAPIToken      = os.Getenv("GRAPH_API_TOKEN")
	port               = os.Getenv("PORT")
)

func main() {
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/", handleRoot)
	log.Printf("Server is listening on port: %s", port)

	// Specify the paths to your certificate and key files
	//certFile := "cert.pem"
	//keyFile := "key.pem"

	// Use ListenAndServeTLS to start the server with HTTPS
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		handleVerification(w, r)
	} else if r.Method == http.MethodPost {
		handleIncomingMessage(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == webhookVerifyToken {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(challenge))
		if err != nil {
			log.Printf("Error writing response: %v", err)
		}
		log.Println("Webhook verified successfully!")
	} else {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}
}

func handleIncomingMessage(w http.ResponseWriter, r *http.Request) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing body: %v", err)
		}
	}(r.Body)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("Incoming webhook message: %s\n", string(body))

	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Extract message data
	entry := reqBody["entry"].([]interface{})[0].(map[string]interface{})
	changes := entry["changes"].([]interface{})[0].(map[string]interface{})
	value := changes["value"].(map[string]interface{})
	log.Printf("Type of messages: %v\n", reflect.TypeOf(value["messages"]))
	if value["messages"] != nil {
		messages := value["messages"].([]interface{})
		if len(messages) > 0 {
			message := messages[0].(map[string]interface{})
			if message["type"] == "text" {
				businessPhoneNumberID := value["metadata"].(map[string]interface{})["phone_number_id"].(string)
				sendReply(businessPhoneNumberID, message)
				markMessageAsRead(businessPhoneNumberID, message["id"].(string))
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func sendReply(businessPhoneNumberID string, message map[string]interface{}) {
	replyBody := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                message["from"],
		"text": map[string]string{
			"body": "Echo: " + message["text"].(map[string]interface{})["body"].(string),
		},
		"context": map[string]string{
			"message_id": message["id"].(string),
		},
	}

	postToWhatsAppAPI(businessPhoneNumberID, replyBody)
}

func markMessageAsRead(businessPhoneNumberID, messageID string) {
	readStatus := map[string]interface{}{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	}

	postToWhatsAppAPI(businessPhoneNumberID, readStatus)
}

func postToWhatsAppAPI(phoneNumberID string, data map[string]interface{}) {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", phoneNumberID)
	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+graphAPIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error posting to WhatsApp API: %v", err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing body: %v", err)
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Error response from WhatsApp API: %s", respBody)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request from %v", r)
	_, err := w.Write([]byte("<pre>Nothing to see here.\nCheckout README.md to start.</pre>"))
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}
