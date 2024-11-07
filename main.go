package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type environmentConfig struct {
	webhookVerifyToken string
	graphAPIToken      string
	port               string
}

func getEnvironmentConfig() environmentConfig {
	return environmentConfig{
		webhookVerifyToken: os.Getenv("WEBHOOK_VERIFY_TOKEN"),
		graphAPIToken:      os.Getenv("GRAPH_API_TOKEN"),
		port:               os.Getenv("PORT"),
	}
}

func handleError(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	log.Println("request %v", r)
	if status == http.StatusBadRequest {
		_, _ = fmt.Fprint(w, "Bad request please check your request body")
	} else if status == http.StatusForbidden {
		_, _ = fmt.Fprint(w, "Forbidden request")
	} else if status == http.StatusMethodNotAllowed {
		_, _ = fmt.Fprint(w, "Method not allowed")
	}
}

func extractMessageData(reqBody map[string]interface{}) (message map[string]interface{}, businessPhoneNumberID string) {
	entry := reqBody["entry"].([]interface{})[0].(map[string]interface{})
	changes := entry["changes"].([]interface{})[0].(map[string]interface{})
	value := changes["value"].(map[string]interface{})
	messages := value["messages"].([]interface{})
	message = nil
	if len(messages) > 0 {
		message = messages[0].(map[string]interface{})
		businessPhoneNumberID = value["metadata"].(map[string]interface{})["phone_number_id"].(string)
	}
	return
}

func getResponseDataForAPI(status string, messageID string, message map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"messaging_product": "whatsapp",
		"status":            status,
		"message_id":        messageID,
	}
	if status == "reply" {
		data["to"] = message["from"]
		data["text"] = map[string]string{
			"body": "Echo: " + message["text"].(map[string]interface{})["body"].(string),
		}
		data["context"] = map[string]string{
			"message_id": messageID,
		}
	}
	return data
}

func postToWhatsAppAPI(envConfig environmentConfig, businessPhoneNumberID string, data map[string]interface{}) {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", businessPhoneNumberID)
	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+envConfig.graphAPIToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error posting to WhatsApp API: %v", err)
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Error response from WhatsApp API: %s", respBody)
	}
}

func fetchTextFromBody(r *http.Request) []byte {
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)
	body, _ := io.ReadAll(r.Body)
	return body
}

func main() {
	envConfig := getEnvironmentConfig()
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleVerification(envConfig, w, r)
		} else if r.Method == http.MethodPost {
			handleIncomingMessage(envConfig, w, r)
		} else {
			handleError(w, r, http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/", handleRoot)

	log.Printf("Server is listening on port: %s", envConfig.port)
	if err := http.ListenAndServe(":"+envConfig.port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func handleVerification(config environmentConfig, w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")
	if mode != "subscribe" || token != config.webhookVerifyToken {
		handleError(w, r, http.StatusForbidden)
	}
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(challenge))
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
	log.Println("Webhook verified successfully!")
}

func handleIncomingMessage(config environmentConfig, w http.ResponseWriter, r *http.Request) {
	body := fetchTextFromBody(r)
	log.Printf("Incoming webhook message: %s\n", string(body))
	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		handleError(w, r, http.StatusBadRequest)
		return
	}

	message, businessPhoneNumberID := extractMessageData(reqBody)
	if message != nil && message["type"] == "text" {
		messageID := message["id"].(string)
		sendReply(config, businessPhoneNumberID, message)
		markMessageAsRead(config, businessPhoneNumberID, messageID)
	}
	w.WriteHeader(http.StatusOK)
}

func sendReply(config environmentConfig, businessPhoneNumberID string, message map[string]interface{}) {
	replyBody := getResponseDataForAPI("reply", message["id"].(string), message)
	postToWhatsAppAPI(config, businessPhoneNumberID, replyBody)
}

func markMessageAsRead(config environmentConfig, businessPhoneNumberID, messageID string) {
	readStatus := getResponseDataForAPI("read", messageID, nil)
	postToWhatsAppAPI(config, businessPhoneNumberID, readStatus)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request from %v", r)
	_, err := w.Write([]byte("<pre>Nothing to see here.\nCheckout README.md to start.</pre>"))
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}
