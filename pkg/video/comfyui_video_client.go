package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ComfyUIVideoClient struct {
	BaseURL        string
	APIKey         string
	Model          string
	Workflow       map[string]interface{}
	ComfyOrgAPIKey string
	HTTPClient     *http.Client
}

type comfyUIVideoSettings struct {
	WorkflowJSON   interface{} `json:"workflow_json"`
	ComfyOrgAPIKey string      `json:"api_key_comfy_org"`
}

func NewComfyUIVideoClient(baseURL, apiKey, model, settings string) (*ComfyUIVideoClient, error) {
	workflow, comfyOrgAPIKey, err := parseComfyVideoWorkflowSettings(settings)
	if err != nil {
		return nil, err
	}
	if comfyOrgAPIKey == "" {
		comfyOrgAPIKey = apiKey
	}
	return &ComfyUIVideoClient{
		BaseURL:        strings.TrimRight(baseURL, "/"),
		APIKey:         apiKey,
		Model:          model,
		Workflow:       workflow,
		ComfyOrgAPIKey: comfyOrgAPIKey,
		HTTPClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}, nil
}

func (c *ComfyUIVideoClient) GenerateVideo(imageURL, prompt string, opts ...VideoOption) (*VideoResult, error) {
	options := &VideoOptions{}
	for _, opt := range opts {
		opt(options)
	}
	workflow := cloneComfyVideoWorkflow(c.Workflow)
	mutateWorkflowForVideo(workflow, prompt, imageURL, c.Model, options)
	payload := map[string]interface{}{
		"prompt": workflow,
	}
	if c.ComfyOrgAPIKey != "" {
		payload["extra_data"] = map[string]interface{}{
			"api_key_comfy_org": c.ComfyOrgAPIKey,
		}
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal comfy payload: %w", err)
	}
	req, err := http.NewRequest("POST", c.BaseURL+"/prompt", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("comfyui API error (status %d): %s", resp.StatusCode, string(body))
	}
	var result struct {
		PromptID string `json:"prompt_id"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("comfyui error: %s", result.Error)
	}
	if result.PromptID == "" {
		return nil, fmt.Errorf("comfyui missing prompt_id")
	}
	return &VideoResult{
		TaskID:    result.PromptID,
		Status:    "processing",
		Completed: false,
	}, nil
}

func (c *ComfyUIVideoClient) GetTaskStatus(taskID string) (*VideoResult, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/history/"+url.PathEscape(taskID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("comfyui API error (status %d): %s", resp.StatusCode, string(body))
	}
	var history map[string]interface{}
	if err := json.Unmarshal(body, &history); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	entryRaw, ok := history[taskID]
	if !ok {
		if queueState, position, queueErr := c.getQueueState(taskID); queueErr == nil {
			if queueState == "queued" {
				return &VideoResult{TaskID: taskID, Status: "queued", Completed: false}, nil
			}
			if queueState == "running" {
				return &VideoResult{TaskID: taskID, Status: "processing", Completed: false}, nil
			}
			if queueState == "missing" && position >= 0 {
				return &VideoResult{TaskID: taskID, Status: "processing", Completed: false}, nil
			}
		}
		return &VideoResult{TaskID: taskID, Status: "processing", Completed: false}, nil
	}
	entry, ok := entryRaw.(map[string]interface{})
	if !ok {
		return &VideoResult{TaskID: taskID, Status: "processing", Completed: false}, nil
	}
	videoURL := extractComfyVideoViewURL(c.BaseURL, entry)
	if videoURL != "" {
		return &VideoResult{
			TaskID:    taskID,
			Status:    "completed",
			VideoURL:  videoURL,
			Completed: true,
		}, nil
	}
	if isComfyVideoTaskCompleted(entry) {
		return &VideoResult{
			TaskID:    taskID,
			Status:    "failed",
			Error:     "任务已完成但未找到视频输出，请检查工作流输出节点",
			Completed: false,
		}, nil
	}
	return &VideoResult{TaskID: taskID, Status: "processing", Completed: false}, nil
}

func (c *ComfyUIVideoClient) getQueueState(taskID string) (string, int, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/queue", nil)
	if err != nil {
		return "unknown", -1, fmt.Errorf("create queue request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "unknown", -1, fmt.Errorf("send queue request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "unknown", -1, fmt.Errorf("read queue response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "unknown", -1, fmt.Errorf("comfyui queue API error (status %d): %s", resp.StatusCode, string(body))
	}
	var queue map[string]interface{}
	if err := json.Unmarshal(body, &queue); err != nil {
		return "unknown", -1, fmt.Errorf("parse queue response: %w", err)
	}
	if pendingRaw, ok := queue["queue_pending"]; ok {
		if position := findComfyVideoTaskPosition(pendingRaw, taskID); position >= 0 {
			return "queued", position, nil
		}
	}
	if runningRaw, ok := queue["queue_running"]; ok {
		if position := findComfyVideoTaskPosition(runningRaw, taskID); position >= 0 {
			return "running", position, nil
		}
	}
	return "missing", -1, nil
}

func findComfyVideoTaskPosition(queueRaw interface{}, taskID string) int {
	queueItems, ok := queueRaw.([]interface{})
	if !ok {
		return -1
	}
	for i, item := range queueItems {
		if extractComfyVideoTaskID(item) == taskID {
			return i
		}
	}
	return -1
}

func extractComfyVideoTaskID(item interface{}) string {
	switch v := item.(type) {
	case []interface{}:
		if len(v) > 1 {
			if id, ok := v[1].(string); ok {
				return id
			}
		}
		if len(v) > 0 {
			if id, ok := v[0].(string); ok {
				return id
			}
		}
	case map[string]interface{}:
		if id, ok := v["prompt_id"].(string); ok {
			return id
		}
		if id, ok := v["id"].(string); ok {
			return id
		}
	}
	return ""
}

func parseComfyVideoWorkflowSettings(settings string) (map[string]interface{}, string, error) {
	if strings.TrimSpace(settings) == "" {
		return nil, "", fmt.Errorf("comfyui settings is empty, expected workflow_json")
	}
	var cfg comfyUIVideoSettings
	if err := json.Unmarshal([]byte(settings), &cfg); err == nil && cfg.WorkflowJSON != nil {
		workflow, err := normalizeComfyVideoWorkflow(cfg.WorkflowJSON)
		if err != nil {
			return nil, "", err
		}
		return workflow, strings.TrimSpace(cfg.ComfyOrgAPIKey), nil
	}
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &generic); err != nil {
		return nil, "", fmt.Errorf("invalid comfyui settings JSON: %w", err)
	}
	if workflowRaw, ok := generic["workflow_json"]; ok {
		workflow, err := normalizeComfyVideoWorkflow(workflowRaw)
		if err != nil {
			return nil, "", err
		}
		apiKey, _ := generic["api_key_comfy_org"].(string)
		return workflow, strings.TrimSpace(apiKey), nil
	}
	workflow, err := normalizeComfyVideoWorkflow(generic)
	if err != nil {
		return nil, "", err
	}
	return workflow, "", nil
}

func normalizeComfyVideoWorkflow(workflowRaw interface{}) (map[string]interface{}, error) {
	switch v := workflowRaw.(type) {
	case string:
		var workflow map[string]interface{}
		if err := json.Unmarshal([]byte(v), &workflow); err != nil {
			return nil, fmt.Errorf("invalid workflow_json string: %w", err)
		}
		return workflow, nil
	case map[string]interface{}:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported workflow_json type")
	}
}

func cloneComfyVideoWorkflow(src map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(src)
	var dst map[string]interface{}
	_ = json.Unmarshal(data, &dst)
	return dst
}

func mutateWorkflowForVideo(node interface{}, prompt string, imageURL string, model string, options *VideoOptions) {
	switch v := node.(type) {
	case map[string]interface{}:
		classType, _ := v["class_type"].(string)
		title := extractComfyVideoNodeTitle(v)
		if inputsRaw, ok := v["inputs"]; ok {
			if inputs, ok := inputsRaw.(map[string]interface{}); ok {
				applyComfyVideoInputs(inputs, classType, title, prompt, imageURL, model, options)
			}
		}
		for key, value := range v {
			if strVal, ok := value.(string); ok {
				replaced := strings.ReplaceAll(strVal, "{{prompt}}", prompt)
				replaced = strings.ReplaceAll(replaced, "{{image_url}}", imageURL)
				if model != "" {
					replaced = strings.ReplaceAll(replaced, "{{model}}", model)
				}
				if options.Duration > 0 {
					replaced = strings.ReplaceAll(replaced, "{{duration}}", strconv.Itoa(options.Duration))
				}
				v[key] = replaced
			}
			mutateWorkflowForVideo(v[key], prompt, imageURL, model, options)
		}
	case []interface{}:
		for i := range v {
			mutateWorkflowForVideo(v[i], prompt, imageURL, model, options)
		}
	}
}

func applyComfyVideoInputs(inputs map[string]interface{}, classType string, title string, prompt string, imageURL string, model string, options *VideoOptions) {
	lowerTitle := strings.ToLower(title)
	lowerClass := strings.ToLower(classType)
	for key, value := range inputs {
		switch key {
		case "seed":
			if options.Seed > 0 {
				inputs[key] = options.Seed
			}
		case "duration":
			if options.Duration > 0 {
				inputs[key] = options.Duration
			}
		case "fps":
			if options.FPS > 0 {
				inputs[key] = options.FPS
			}
		case "image", "image_url", "prompt_image", "first_frame", "first_frame_image":
			if imageURL != "" {
				if _, ok := value.(string); ok {
					inputs[key] = imageURL
				}
			}
		case "last_frame", "last_frame_image":
			if options.LastFrameURL != "" {
				if _, ok := value.(string); ok {
					inputs[key] = options.LastFrameURL
				}
			}
		case "value":
			if strings.Contains(lowerClass, "primitivestring") {
				if _, ok := value.(string); ok {
					inputs[key] = prompt
				}
			}
		case "text", "prompt", "prompt_text":
			if _, ok := value.(string); ok {
				if strings.Contains(lowerTitle, "negative") {
					continue
				}
				if strings.Contains(lowerClass, "cliptextencode") || strings.Contains(lowerClass, "text") || key != "text" {
					inputs[key] = prompt
				}
			}
		}
		if strVal, ok := value.(string); ok {
			replaced := strings.ReplaceAll(strVal, "{{prompt}}", prompt)
			replaced = strings.ReplaceAll(replaced, "{{image_url}}", imageURL)
			if model != "" {
				replaced = strings.ReplaceAll(replaced, "{{model}}", model)
			}
			if options.Duration > 0 {
				replaced = strings.ReplaceAll(replaced, "{{duration}}", strconv.Itoa(options.Duration))
			}
			inputs[key] = replaced
		}
	}
}

func extractComfyVideoNodeTitle(node map[string]interface{}) string {
	metaRaw, ok := node["_meta"]
	if !ok {
		return ""
	}
	meta, ok := metaRaw.(map[string]interface{})
	if !ok {
		return ""
	}
	title, _ := meta["title"].(string)
	return title
}

func extractComfyVideoViewURL(baseURL string, entry map[string]interface{}) string {
	outputsRaw, ok := entry["outputs"]
	if !ok {
		return ""
	}
	outputs, ok := outputsRaw.(map[string]interface{})
	if !ok {
		return ""
	}
	for _, nodeOutputRaw := range outputs {
		nodeOutput, ok := nodeOutputRaw.(map[string]interface{})
		if !ok {
			continue
		}
		mediaItemsRaw, ok := nodeOutput["videos"]
		if !ok {
			continue
		}
		mediaItems, ok := mediaItemsRaw.([]interface{})
		if !ok || len(mediaItems) == 0 {
			continue
		}
		first, ok := mediaItems[0].(map[string]interface{})
		if !ok {
			continue
		}
		filename, _ := first["filename"].(string)
		if filename == "" {
			if directURL, ok := first["url"].(string); ok {
				return directURL
			}
			continue
		}
		subfolder, _ := first["subfolder"].(string)
		fileType, _ := first["type"].(string)
		query := url.Values{}
		query.Set("filename", filename)
		query.Set("subfolder", subfolder)
		if fileType != "" {
			query.Set("type", fileType)
		} else {
			query.Set("type", "output")
		}
		return baseURL + "/view?" + query.Encode()
	}
	return ""
}

func isComfyVideoTaskCompleted(entry map[string]interface{}) bool {
	statusRaw, ok := entry["status"]
	if !ok {
		return false
	}
	statusMap, ok := statusRaw.(map[string]interface{})
	if !ok {
		return false
	}
	if completedRaw, ok := statusMap["completed"]; ok {
		if completed, ok := completedRaw.(bool); ok {
			return completed
		}
	}
	return false
}
