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
		for key, value := range v {
			switch key {
			case "prompt", "prompt_text":
				if _, ok := value.(string); ok {
					v[key] = prompt
				}
			case "image", "image_url", "prompt_image", "first_frame", "first_frame_image":
				if imageURL != "" {
					v[key] = imageURL
				}
			case "last_frame", "last_frame_image":
				if options.LastFrameURL != "" {
					v[key] = options.LastFrameURL
				}
			case "model":
				if model != "" {
					v[key] = model
				}
			case "seed":
				if options.Seed > 0 {
					v[key] = options.Seed
				}
			case "duration":
				if options.Duration > 0 {
					v[key] = options.Duration
				}
			case "fps":
				if options.FPS > 0 {
					v[key] = options.FPS
				}
			default:
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
			}
			mutateWorkflowForVideo(v[key], prompt, imageURL, model, options)
		}
	case []interface{}:
		for i := range v {
			mutateWorkflowForVideo(v[i], prompt, imageURL, model, options)
		}
	}
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
