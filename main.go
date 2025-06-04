package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Team struct {
	Name        string            `json:"name"`
	Keywords    []string          `json:"keywords"`
	Exclusions  []string          `json:"exclusions"`
	Tags        []string          `json:"tags"`
	Description string            `json:"description"`
	Contacts    map[string]string `json:"contacts"`
	Examples    []string          `json:"examples"`
}

type KnowledgeBase struct {
	Teams             []Team            `json:"teams"`
	ResponseTemplates map[string]string `json:"response_template"`
}

type DeepSeekRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var (
	kb          KnowledgeBase
	deepSeekURL = "https://api.deepseek.com/v1/chat/completions"
)

func main() {
	// Загрузка .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env не загружен:", err)
	}

	// Загрузка базы знаний
	if err := loadKnowledgeBase("knowledge_base.json"); err != nil {
		log.Fatal("❌ Ошибка загрузки базы знаний:", err)
	}

	// CLI интерфейс
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("🤖 AI-агент техподдержки (JSON+RAG)")
	fmt.Println("Введите запрос (или 'выход'):")

	for {
		fmt.Print("> ")
		query, _ := reader.ReadString('\n')
		query = strings.TrimSpace(query)

		if shouldExit(query) {
			break
		}

		processQuery(query)
	}
}

func loadKnowledgeBase(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &kb); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	log.Printf("✅ Загружено команд: %d", len(kb.Teams))
	return nil
}

func processQuery(query string) {
	// Формируем контекст для DeepSeek
	context := struct {
		KnowledgeBase KnowledgeBase `json:"knowledge_base"`
		UserQuery     string        `json:"user_query"`
	}{
		KnowledgeBase: kb,
		UserQuery:     query,
	}

	contextJSON, _ := json.MarshalIndent(context, "", "  ")
	fullPrompt := fmt.Sprintf(`
Анализируй запрос используя ТОЛЬКО эту базу знаний:
%s

Формат ответа:
- Если запрос относится к конкретной команде: %s
- Если не удалось определить: %s`,
		string(contextJSON),
		kb.ResponseTemplates["success"],
		kb.ResponseTemplates["unknown"])

	// Отправка в DeepSeek
	response, err := askDeepSeek(fullPrompt)
	if err != nil {
		log.Println("❌ Ошибка:", err)
		return
	}

	fmt.Println("\n🤖 Ответ:")
	fmt.Println(response)
}

func askDeepSeek(prompt string) (string, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("DEEPSEEK_API_KEY не найден")
	}

	requestBody := DeepSeekRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{
				Role:    "system",
				Content: "Ты ИИ-ассистент техподдержки. Отвечай строго по предоставленной базе знаний.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonBody, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", deepSeekURL, bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Только для теста!
		},
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API ошибка %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ API")
	}

	return result.Choices[0].Message.Content, nil
}

func shouldExit(query string) bool {
	return strings.ToLower(query) == "выход"
}
