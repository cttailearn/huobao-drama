package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/drama-generator/backend/domain/models"
	"github.com/drama-generator/backend/pkg/ai"
	"github.com/drama-generator/backend/pkg/logger"
	"gorm.io/gorm"
)

type AIService struct {
	db  *gorm.DB
	log *logger.Logger
}

func NewAIService(db *gorm.DB, log *logger.Logger) *AIService {
	return &AIService{
		db:  db,
		log: log,
	}
}

type CreateAIConfigRequest struct {
	ServiceType   string            `json:"service_type" binding:"required,oneof=text image video"`
	Name          string            `json:"name" binding:"required,min=1,max=100"`
	Provider      string            `json:"provider" binding:"required"`
	BaseURL       string            `json:"base_url" binding:"required,url"`
	APIKey        string            `json:"api_key"`
	Model         models.ModelField `json:"model"`
	Endpoint      string            `json:"endpoint"`
	QueryEndpoint string            `json:"query_endpoint"`
	Priority      int               `json:"priority"`
	IsDefault     bool              `json:"is_default"`
	Settings      string            `json:"settings"`
}

type UpdateAIConfigRequest struct {
	Name          string             `json:"name" binding:"omitempty,min=1,max=100"`
	Provider      string             `json:"provider"`
	BaseURL       string             `json:"base_url" binding:"omitempty,url"`
	APIKey        string             `json:"api_key"`
	Model         *models.ModelField `json:"model"`
	Endpoint      string             `json:"endpoint"`
	QueryEndpoint string             `json:"query_endpoint"`
	Priority      *int               `json:"priority"`
	IsDefault     bool               `json:"is_default"`
	IsActive      bool               `json:"is_active"`
	Settings      string             `json:"settings"`
}

type TestConnectionRequest struct {
	BaseURL  string            `json:"base_url" binding:"required,url"`
	APIKey   string            `json:"api_key"`
	Model    models.ModelField `json:"model"`
	Provider string            `json:"provider"`
	Endpoint string            `json:"endpoint"`
}

type ValidateWorkflowRequest struct {
	Provider    string `json:"provider" binding:"required"`
	ServiceType string `json:"service_type" binding:"required,oneof=image video"`
	Settings    string `json:"settings" binding:"required"`
}

type ValidateWorkflowResult struct {
	Valid               bool     `json:"valid"`
	PromptNodeCount     int      `json:"prompt_node_count"`
	OutputNodeCount     int      `json:"output_node_count"`
	PromptNodeHints     []string `json:"prompt_node_hints"`
	OutputNodeHints     []string `json:"output_node_hints"`
	MissingRequirements []string `json:"missing_requirements"`
}

func (s *AIService) CreateConfig(req *CreateAIConfigRequest) (*models.AIServiceConfig, error) {
	if req.Provider != "comfyui" && strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("api_key is required for provider %s", req.Provider)
	}
	if req.Provider != "comfyui" && len(req.Model) == 0 {
		return nil, fmt.Errorf("model is required for provider %s", req.Provider)
	}
	if req.Provider == "comfyui" && (req.ServiceType == "image" || req.ServiceType == "video") {
		if err := validateComfySettings(req.Settings); err != nil {
			return nil, err
		}
	}
	// 根据 provider 和 service_type 自动设置 endpoint
	endpoint := req.Endpoint
	queryEndpoint := req.QueryEndpoint

	if endpoint == "" {
		switch req.Provider {
		case "gemini", "google":
			if req.ServiceType == "text" {
				endpoint = "/v1beta/models/{model}:generateContent"
			} else if req.ServiceType == "image" {
				endpoint = "/v1beta/models/{model}:generateContent"
			}
		case "openai":
			if req.ServiceType == "text" {
				endpoint = "/chat/completions"
			} else if req.ServiceType == "image" {
				endpoint = "/images/generations"
			} else if req.ServiceType == "video" {
				endpoint = "/videos"
				if queryEndpoint == "" {
					queryEndpoint = "/videos/{taskId}"
				}
			}
		case "chatfire":
			if req.ServiceType == "text" {
				endpoint = "/chat/completions"
			} else if req.ServiceType == "image" {
				endpoint = "/images/generations"
			} else if req.ServiceType == "video" {
				endpoint = "/video/generations"
				if queryEndpoint == "" {
					queryEndpoint = "/video/task/{taskId}"
				}
			}
		case "doubao", "volcengine", "volces":
			if req.ServiceType == "video" {
				endpoint = "/contents/generations/tasks"
				if queryEndpoint == "" {
					queryEndpoint = "/generations/tasks/{taskId}"
				}
			}
		case "comfyui":
			if req.ServiceType == "image" || req.ServiceType == "video" {
				endpoint = "/prompt"
				if queryEndpoint == "" {
					queryEndpoint = "/history/{taskId}"
				}
			}
		default:
			// 默认使用 OpenAI 格式
			if req.ServiceType == "text" {
				endpoint = "/chat/completions"
			} else if req.ServiceType == "image" {
				endpoint = "/images/generations"
			}
		}
	}

	config := &models.AIServiceConfig{
		ServiceType:   req.ServiceType,
		Name:          req.Name,
		Provider:      req.Provider,
		BaseURL:       req.BaseURL,
		APIKey:        req.APIKey,
		Model:         req.Model,
		Endpoint:      endpoint,
		QueryEndpoint: queryEndpoint,
		Priority:      req.Priority,
		IsDefault:     req.IsDefault,
		IsActive:      true,
		Settings:      req.Settings,
	}

	if err := s.db.Create(config).Error; err != nil {
		s.log.Errorw("Failed to create AI config", "error", err)
		return nil, err
	}

	s.log.Infow("AI config created", "config_id", config.ID, "provider", req.Provider, "endpoint", endpoint)
	return config, nil
}

func (s *AIService) GetConfig(configID uint) (*models.AIServiceConfig, error) {
	var config models.AIServiceConfig
	err := s.db.Where("id = ? ", configID).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("config not found")
		}
		return nil, err
	}
	return &config, nil
}

func (s *AIService) ListConfigs(serviceType string) ([]models.AIServiceConfig, error) {
	var configs []models.AIServiceConfig
	query := s.db

	if serviceType != "" {
		query = query.Where("service_type = ?", serviceType)
	}

	err := query.Order("priority DESC, created_at DESC").Find(&configs).Error
	if err != nil {
		s.log.Errorw("Failed to list AI configs", "error", err)
		return nil, err
	}

	return configs, nil
}

func (s *AIService) UpdateConfig(configID uint, req *UpdateAIConfigRequest) (*models.AIServiceConfig, error) {
	var config models.AIServiceConfig
	if err := s.db.Where("id = ? ", configID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("config not found")
		}
		return nil, err
	}

	tx := s.db.Begin()

	// 不再需要is_default独占逻辑

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Provider != "" {
		updates["provider"] = req.Provider
	}
	if req.BaseURL != "" {
		updates["base_url"] = req.BaseURL
	}
	if req.APIKey != "" {
		updates["api_key"] = req.APIKey
	}
	if req.Model != nil && len(*req.Model) > 0 {
		updates["model"] = *req.Model
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}

	// 如果提供了 provider，根据 provider 和 service_type 自动设置 endpoint
	if req.Provider != "" && req.Endpoint == "" {
		provider := req.Provider
		serviceType := config.ServiceType

		switch provider {
		case "gemini", "google":
			if serviceType == "text" || serviceType == "image" {
				updates["endpoint"] = "/v1beta/models/{model}:generateContent"
			}
		case "openai":
			if serviceType == "text" {
				updates["endpoint"] = "/chat/completions"
			} else if serviceType == "image" {
				updates["endpoint"] = "/images/generations"
			} else if serviceType == "video" {
				updates["endpoint"] = "/videos"
				updates["query_endpoint"] = "/videos/{taskId}"
			}
		case "chatfire":
			if serviceType == "text" {
				updates["endpoint"] = "/chat/completions"
			} else if serviceType == "image" {
				updates["endpoint"] = "/images/generations"
			} else if serviceType == "video" {
				updates["endpoint"] = "/video/generations"
				updates["query_endpoint"] = "/video/task/{taskId}"
			}
		case "comfyui":
			if serviceType == "image" || serviceType == "video" {
				updates["endpoint"] = "/prompt"
				updates["query_endpoint"] = "/history/{taskId}"
			}
		}
	} else if req.Endpoint != "" {
		updates["endpoint"] = req.Endpoint
	}

	// 允许清空query_endpoint，所以不检查是否为空
	updates["query_endpoint"] = req.QueryEndpoint
	if req.Settings != "" {
		targetProvider := config.Provider
		if req.Provider != "" {
			targetProvider = req.Provider
		}
		if targetProvider == "comfyui" && (config.ServiceType == "image" || config.ServiceType == "video") {
			if err := validateComfySettings(req.Settings); err != nil {
				return nil, err
			}
		}
		updates["settings"] = req.Settings
	}
	updates["is_default"] = req.IsDefault
	updates["is_active"] = req.IsActive

	if err := tx.Model(&config).Updates(updates).Error; err != nil {
		tx.Rollback()
		s.log.Errorw("Failed to update AI config", "error", err)
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.log.Infow("AI config updated", "config_id", configID)
	return &config, nil
}

func (s *AIService) DeleteConfig(configID uint) error {
	result := s.db.Where("id = ? ", configID).Delete(&models.AIServiceConfig{})

	if result.Error != nil {
		s.log.Errorw("Failed to delete AI config", "error", result.Error)
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("config not found")
	}

	s.log.Infow("AI config deleted", "config_id", configID)
	return nil
}

func (s *AIService) TestConnection(req *TestConnectionRequest) error {
	s.log.Infow("TestConnection called", "baseURL", req.BaseURL, "provider", req.Provider, "endpoint", req.Endpoint, "modelCount", len(req.Model))
	if req.Provider == "comfyui" {
		endpoint := strings.TrimRight(req.BaseURL, "/") + "/system_stats"
		httpClient := &http.Client{Timeout: 15 * time.Second}
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			s.log.Errorw("ComfyUI test connection failed", "error", err, "endpoint", endpoint)
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("comfyui test failed with status %d", resp.StatusCode)
		}
		return nil
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return fmt.Errorf("api_key is required")
	}

	// 使用第一个模型进行测试
	model := ""
	if len(req.Model) > 0 {
		model = req.Model[0]
	}
	s.log.Infow("Using model for test", "model", model, "provider", req.Provider)

	// 根据 provider 参数选择客户端
	var client ai.AIClient
	var endpoint string

	switch req.Provider {
	case "gemini", "google":
		// Gemini
		s.log.Infow("Using Gemini client", "baseURL", req.BaseURL)
		endpoint = "/v1beta/models/{model}:generateContent"
		client = ai.NewGeminiClient(req.BaseURL, req.APIKey, model, endpoint)
	case "openai", "chatfire":
		// OpenAI 格式（包括 chatfire 等）
		s.log.Infow("Using OpenAI-compatible client", "baseURL", req.BaseURL, "provider", req.Provider)
		endpoint = req.Endpoint
		if endpoint == "" {
			endpoint = "/chat/completions"
		}
		client = ai.NewOpenAIClient(req.BaseURL, req.APIKey, model, endpoint)
	default:
		// 默认使用 OpenAI 格式
		s.log.Infow("Using default OpenAI-compatible client", "baseURL", req.BaseURL)
		endpoint = req.Endpoint
		if endpoint == "" {
			endpoint = "/chat/completions"
		}
		client = ai.NewOpenAIClient(req.BaseURL, req.APIKey, model, endpoint)
	}

	s.log.Infow("Calling TestConnection on client", "endpoint", endpoint)
	err := client.TestConnection()
	if err != nil {
		s.log.Errorw("TestConnection failed", "error", err)
	} else {
		s.log.Infow("TestConnection succeeded")
	}
	return err
}

func validateComfySettings(settings string) error {
	_, err := parseComfyWorkflowSettings(settings)
	return err
}

func parseComfyWorkflowSettings(settings string) (map[string]interface{}, error) {
	if strings.TrimSpace(settings) == "" {
		return nil, fmt.Errorf("comfyui settings is empty, expected workflow_json")
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &raw); err != nil {
		return nil, fmt.Errorf("invalid comfyui settings JSON: %w", err)
	}
	workflowRaw, hasWorkflow := raw["workflow_json"]
	if !hasWorkflow {
		workflowRaw = raw
	}
	switch v := workflowRaw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, fmt.Errorf("comfyui workflow_json is empty")
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("invalid comfyui workflow_json string: %w", err)
		}
		if len(parsed) == 0 {
			return nil, fmt.Errorf("comfyui workflow_json is empty")
		}
		return parsed, nil
	case map[string]interface{}:
		if len(v) == 0 {
			return nil, fmt.Errorf("comfyui workflow_json is empty")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported workflow_json type")
	}
}

func (s *AIService) ValidateWorkflow(req *ValidateWorkflowRequest) (*ValidateWorkflowResult, error) {
	if strings.TrimSpace(req.Provider) != "comfyui" {
		return nil, fmt.Errorf("当前仅支持 ComfyUI 工作流自检")
	}
	workflow, err := parseComfyWorkflowSettings(req.Settings)
	if err != nil {
		return nil, err
	}
	result := &ValidateWorkflowResult{
		Valid:               true,
		PromptNodeHints:     make([]string, 0),
		OutputNodeHints:     make([]string, 0),
		MissingRequirements: make([]string, 0),
	}
	for _, nodeRaw := range workflow {
		node, ok := nodeRaw.(map[string]interface{})
		if !ok {
			continue
		}
		classType := strings.ToLower(strings.TrimSpace(getComfyNodeClassType(node)))
		title := strings.ToLower(strings.TrimSpace(getComfyNodeTitle(node)))
		inputs := getComfyNodeInputs(node)

		if hasWritablePromptInput(inputs, classType, title) {
			result.PromptNodeCount++
			result.PromptNodeHints = append(result.PromptNodeHints, getComfyNodeHint(node, classType))
		}
		if req.ServiceType == "image" && isComfyImageOutputNode(classType, title, inputs) {
			result.OutputNodeCount++
			result.OutputNodeHints = append(result.OutputNodeHints, getComfyNodeHint(node, classType))
		}
		if req.ServiceType == "video" && isComfyVideoOutputNode(classType, title, inputs) {
			result.OutputNodeCount++
			result.OutputNodeHints = append(result.OutputNodeHints, getComfyNodeHint(node, classType))
		}
	}
	if result.PromptNodeCount == 0 {
		result.Valid = false
		result.MissingRequirements = append(result.MissingRequirements, "未找到可写提示词节点（如 CLIPTextEncode.text 或 PrimitiveStringMultiline.value）")
	}
	if result.OutputNodeCount == 0 {
		result.Valid = false
		if req.ServiceType == "image" {
			result.MissingRequirements = append(result.MissingRequirements, "未找到图片输出节点（如 SaveImage/PreviewImage 或输出 images）")
		} else {
			result.MissingRequirements = append(result.MissingRequirements, "未找到视频输出节点（如 VHS_VideoCombine/SaveVideo 或输出 videos）")
		}
	}
	return result, nil
}

func getComfyNodeClassType(node map[string]interface{}) string {
	classType, _ := node["class_type"].(string)
	return classType
}

func getComfyNodeTitle(node map[string]interface{}) string {
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

func getComfyNodeInputs(node map[string]interface{}) map[string]interface{} {
	inputsRaw, ok := node["inputs"]
	if !ok {
		return map[string]interface{}{}
	}
	inputs, ok := inputsRaw.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return inputs
}

func hasWritablePromptInput(inputs map[string]interface{}, classType string, title string) bool {
	if strings.Contains(title, "negative") {
		return false
	}
	if value, ok := inputs["text"]; ok {
		if _, isString := value.(string); isString {
			return strings.Contains(classType, "cliptextencode") || strings.Contains(classType, "text")
		}
	}
	if value, ok := inputs["value"]; ok {
		if _, isString := value.(string); isString && strings.Contains(classType, "primitivestring") {
			return true
		}
	}
	return false
}

func isComfyImageOutputNode(classType string, title string, inputs map[string]interface{}) bool {
	if strings.Contains(classType, "saveimage") || strings.Contains(classType, "previewimage") {
		return true
	}
	if strings.Contains(title, "saveimage") || strings.Contains(title, "previewimage") {
		return true
	}
	_, hasImages := inputs["images"]
	return hasImages
}

func isComfyVideoOutputNode(classType string, title string, inputs map[string]interface{}) bool {
	if strings.Contains(classType, "video") || strings.Contains(classType, "savevideo") || strings.Contains(classType, "vhs_videocombine") {
		return true
	}
	if strings.Contains(title, "video") || strings.Contains(title, "save video") {
		return true
	}
	_, hasVideos := inputs["videos"]
	return hasVideos
}

func getComfyNodeHint(node map[string]interface{}, classType string) string {
	nodeID, _ := node["id"].(string)
	title := getComfyNodeTitle(node)
	if title != "" {
		if nodeID != "" {
			return fmt.Sprintf("%s (%s)", title, nodeID)
		}
		return title
	}
	if nodeID != "" {
		return fmt.Sprintf("%s (%s)", classType, nodeID)
	}
	return classType
}

func (s *AIService) GetDefaultConfig(serviceType string) (*models.AIServiceConfig, error) {
	var config models.AIServiceConfig
	// 按优先级降序获取第一个激活的配置
	err := s.db.Where("service_type = ? AND is_active = ?", serviceType, true).
		Order("priority DESC, created_at DESC").
		First(&config).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no active config found")
		}
		return nil, err
	}

	return &config, nil
}

// GetConfigForModel 根据服务类型和模型名称获取优先级最高的激活配置
func (s *AIService) GetConfigForModel(serviceType string, modelName string) (*models.AIServiceConfig, error) {
	var configs []models.AIServiceConfig
	err := s.db.Where("service_type = ? AND is_active = ?", serviceType, true).
		Order("priority DESC, created_at DESC").
		Find(&configs).Error

	if err != nil {
		return nil, err
	}

	// 查找包含指定模型的配置
	for _, config := range configs {
		for _, model := range config.Model {
			if model == modelName {
				return &config, nil
			}
		}
	}

	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	if normalizedModelName != "" {
		for _, config := range configs {
			if strings.ToLower(strings.TrimSpace(config.Provider)) == normalizedModelName {
				return &config, nil
			}
		}
	}

	return nil, errors.New("no active config found for model: " + modelName)
}

func (s *AIService) GetAIClient(serviceType string) (ai.AIClient, error) {
	config, err := s.GetDefaultConfig(serviceType)
	if err != nil {
		return nil, err
	}

	// 使用第一个模型
	model := ""
	if len(config.Model) > 0 {
		model = config.Model[0]
	}

	// 使用数据库配置中的 endpoint，如果为空则根据 provider 设置默认值
	endpoint := config.Endpoint
	if endpoint == "" {
		switch config.Provider {
		case "gemini", "google":
			endpoint = "/v1beta/models/{model}:generateContent"
		default:
			endpoint = "/chat/completions"
		}
	}

	// 根据 provider 创建对应的客户端
	switch config.Provider {
	case "gemini", "google":
		return ai.NewGeminiClient(config.BaseURL, config.APIKey, model, endpoint), nil
	default:
		// openai, chatfire 等其他厂商都使用 OpenAI 格式
		return ai.NewOpenAIClient(config.BaseURL, config.APIKey, model, endpoint), nil
	}
}

// GetAIClientForModel 根据服务类型和模型名称获取对应的AI客户端
func (s *AIService) GetAIClientForModel(serviceType string, modelName string) (ai.AIClient, error) {
	config, err := s.GetConfigForModel(serviceType, modelName)
	if err != nil {
		return nil, err
	}

	// 使用数据库配置中的 endpoint，如果为空则根据 provider 设置默认值
	endpoint := config.Endpoint
	if endpoint == "" {
		switch config.Provider {
		case "gemini", "google":
			endpoint = "/v1beta/models/{model}:generateContent"
		default:
			endpoint = "/chat/completions"
		}
	}

	// 根据 provider 创建对应的客户端
	switch config.Provider {
	case "gemini", "google":
		return ai.NewGeminiClient(config.BaseURL, config.APIKey, modelName, endpoint), nil
	default:
		// openai, chatfire 等其他厂商都使用 OpenAI 格式
		return ai.NewOpenAIClient(config.BaseURL, config.APIKey, modelName, endpoint), nil
	}
}

func (s *AIService) GenerateText(prompt string, systemPrompt string, options ...func(*ai.ChatCompletionRequest)) (string, error) {
	client, err := s.GetAIClient("text")
	if err != nil {
		return "", fmt.Errorf("failed to get AI client: %w", err)
	}

	return client.GenerateText(prompt, systemPrompt, options...)
}

func (s *AIService) GenerateImage(prompt string, size string, n int) ([]string, error) {
	client, err := s.GetAIClient("image")
	if err != nil {
		return nil, fmt.Errorf("failed to get AI client for image: %w", err)
	}

	return client.GenerateImage(prompt, size, n)
}
