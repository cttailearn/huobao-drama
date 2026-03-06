package image

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

type ComfyUIImageClient struct {
	BaseURL        string
	APIKey         string
	Model          string
	Workflow       map[string]interface{}
	ComfyOrgAPIKey string
	HTTPClient     *http.Client
}

type comfyUISettings struct {
	WorkflowJSON   interface{} `json:"workflow_json"`
	ComfyOrgAPIKey string      `json:"api_key_comfy_org"`
}

func NewComfyUIImageClient(baseURL, apiKey, model, settings string) (*ComfyUIImageClient, error) {
	workflow, comfyOrgAPIKey, err := parseComfyWorkflowSettings(settings)
	if err != nil {
		return nil, err
	}
	if comfyOrgAPIKey == "" {
		comfyOrgAPIKey = apiKey
	}
	return &ComfyUIImageClient{
		BaseURL:        strings.TrimRight(baseURL, "/"),
		APIKey:         apiKey,
		Model:          model,
		Workflow:       workflow,
		ComfyOrgAPIKey: comfyOrgAPIKey,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (c *ComfyUIImageClient) GenerateImage(prompt string, opts ...ImageOption) (*ImageResult, error) {
	options := &ImageOptions{}
	for _, opt := range opts {
		opt(options)
	}
	workflow := cloneWorkflow(c.Workflow)
	mutateWorkflowForImage(workflow, prompt, c.Model, options)
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
	return &ImageResult{
		TaskID:    result.PromptID,
		Status:    "processing",
		Completed: false,
	}, nil
}

func (c *ComfyUIImageClient) GetTaskStatus(taskID string) (*ImageResult, error) {
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
		return &ImageResult{TaskID: taskID, Status: "processing", Completed: false}, nil
	}
	entry, ok := entryRaw.(map[string]interface{})
	if !ok {
		return &ImageResult{TaskID: taskID, Status: "processing", Completed: false}, nil
	}
	imageURL := extractComfyViewURL(c.BaseURL, entry, "images")
	if imageURL != "" {
		return &ImageResult{
			TaskID:    taskID,
			Status:    "completed",
			ImageURL:  imageURL,
			Completed: true,
		}, nil
	}
	if isComfyTaskCompleted(entry) {
		return &ImageResult{
			TaskID:    taskID,
			Status:    "failed",
			Error:     "任务已完成但未找到图片输出，请检查工作流输出节点",
			Completed: false,
		}, nil
	}
	return &ImageResult{TaskID: taskID, Status: "processing", Completed: false}, nil
}

func parseComfyWorkflowSettings(settings string) (map[string]interface{}, string, error) {
	if strings.TrimSpace(settings) == "" {
		return nil, "", fmt.Errorf("comfyui settings is empty, expected workflow_json")
	}
	var cfg comfyUISettings
	if err := json.Unmarshal([]byte(settings), &cfg); err == nil && cfg.WorkflowJSON != nil {
		workflow, err := normalizeWorkflow(cfg.WorkflowJSON)
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
		workflow, err := normalizeWorkflow(workflowRaw)
		if err != nil {
			return nil, "", err
		}
		apiKey, _ := generic["api_key_comfy_org"].(string)
		return workflow, strings.TrimSpace(apiKey), nil
	}
	workflow, err := normalizeWorkflow(generic)
	if err != nil {
		return nil, "", err
	}
	return workflow, "", nil
}

func normalizeWorkflow(workflowRaw interface{}) (map[string]interface{}, error) {
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

func cloneWorkflow(src map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(src)
	var dst map[string]interface{}
	_ = json.Unmarshal(data, &dst)
	return dst
}

func mutateWorkflowForImage(node interface{}, prompt string, model string, options *ImageOptions) {
	switch v := node.(type) {
	case map[string]interface{}:
		for key, value := range v {
			switch key {
			case "prompt", "prompt_text":
				if _, ok := value.(string); ok {
					v[key] = prompt
				}
			case "model":
				if model != "" {
					v[key] = model
				}
			case "width":
				if options.Width > 0 {
					v[key] = options.Width
				}
			case "height":
				if options.Height > 0 {
					v[key] = options.Height
				}
			case "seed":
				if options.Seed > 0 {
					v[key] = options.Seed
				}
			default:
				if strVal, ok := value.(string); ok {
					v[key] = strings.ReplaceAll(strVal, "{{prompt}}", prompt)
					if model != "" {
						v[key] = strings.ReplaceAll(v[key].(string), "{{model}}", model)
					}
					if options.Width > 0 {
						v[key] = strings.ReplaceAll(v[key].(string), "{{width}}", strconv.Itoa(options.Width))
					}
					if options.Height > 0 {
						v[key] = strings.ReplaceAll(v[key].(string), "{{height}}", strconv.Itoa(options.Height))
					}
				}
			}
			mutateWorkflowForImage(v[key], prompt, model, options)
		}
	case []interface{}:
		for i := range v {
			mutateWorkflowForImage(v[i], prompt, model, options)
		}
	}
}

func extractComfyViewURL(baseURL string, entry map[string]interface{}, mediaKey string) string {
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
		mediaItemsRaw, ok := nodeOutput[mediaKey]
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

func isComfyTaskCompleted(entry map[string]interface{}) bool {
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
