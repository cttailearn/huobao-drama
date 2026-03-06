package services

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/drama-generator/backend/domain/models"
	"github.com/drama-generator/backend/pkg/ai"
	"github.com/drama-generator/backend/pkg/config"
	"github.com/drama-generator/backend/pkg/logger"
	"github.com/drama-generator/backend/pkg/utils"
	"gorm.io/gorm"
)

type NovelService struct {
	db          *gorm.DB
	aiService   *AIService
	taskService *TaskService
	log         *logger.Logger
}

type CreateNovelRequest struct {
	DramaID         uint   `json:"drama_id" binding:"required"`
	Title           string `json:"title" binding:"required,min=1,max=200"`
	Genre           string `json:"genre" binding:"required,min=1,max=100"`
	ChapterCount    int    `json:"chapter_count" binding:"required,min=1,max=200"`
	WordsPerChapter int    `json:"words_per_chapter" binding:"required,min=200,max=12000"`
	Requirement     string `json:"requirement" binding:"max=2000"`
}

type ListNovelQuery struct {
	Page     int  `form:"page,default=1"`
	PageSize int  `form:"page_size,default=20"`
	DramaID  uint `form:"drama_id"`
}

type ChapterOutlineItem struct {
	ChapterNumber int    `json:"chapter_number"`
	Title         string `json:"title"`
	Outline       string `json:"outline"`
}

type UpdateNovelContentRequest struct {
	SetupContent   string             `json:"setup_content"`
	OutlineContent string             `json:"outline_content"`
	Chapters       []ChapterEditInput `json:"chapters"`
}

type ChapterEditInput struct {
	ChapterNumber int    `json:"chapter_number"`
	Title         string `json:"title"`
	Outline       string `json:"outline"`
}

type UpdateChapterContentRequest struct {
	DraftContent string `json:"draft_content"`
	FinalContent string `json:"final_content"`
}

func NewNovelService(db *gorm.DB, cfg *config.Config, log *logger.Logger) *NovelService {
	_ = cfg
	return &NovelService{
		db:          db,
		aiService:   NewAIService(db, log),
		taskService: NewTaskService(db, log),
		log:         log,
	}
}

func (s *NovelService) CreateNovel(req *CreateNovelRequest) (*models.Novel, error) {
	var drama models.Drama
	if err := s.db.Where("id = ?", req.DramaID).First(&drama).Error; err != nil {
		return nil, fmt.Errorf("drama project not found")
	}
	novel := &models.Novel{
		DramaID:         req.DramaID,
		Title:           strings.TrimSpace(req.Title),
		Genre:           strings.TrimSpace(req.Genre),
		ChapterCount:    req.ChapterCount,
		WordsPerChapter: req.WordsPerChapter,
		Requirement:     strings.TrimSpace(req.Requirement),
		Status:          "draft",
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(novel).Error; err != nil {
			return err
		}
		chapters := make([]models.NovelChapter, 0, req.ChapterCount)
		for i := 1; i <= req.ChapterCount; i++ {
			chapters = append(chapters, models.NovelChapter{
				NovelID:       novel.ID,
				ChapterNumber: i,
				Title:         fmt.Sprintf("第%d章", i),
				Status:        "pending",
			})
		}
		if len(chapters) > 0 {
			if err := tx.Create(&chapters).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetNovel(novel.ID)
}

func (s *NovelService) ListNovels(query *ListNovelQuery) ([]models.Novel, int64, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	var novels []models.Novel
	var total int64
	db := s.db.Model(&models.Novel{})
	if query.DramaID > 0 {
		db = db.Where("drama_id = ?", query.DramaID)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (query.Page - 1) * query.PageSize
	err := db.Order("updated_at DESC").
		Offset(offset).
		Limit(query.PageSize).
		Preload("Chapters", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("chapter_number ASC")
		}).
		Find(&novels).Error
	if err != nil {
		return nil, 0, err
	}
	return novels, total, nil
}

func (s *NovelService) GetNovel(id uint) (*models.Novel, error) {
	var novel models.Novel
	if err := s.db.Where("id = ?", id).
		Preload("Chapters", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("chapter_number ASC")
		}).
		First(&novel).Error; err != nil {
		return nil, err
	}
	return &novel, nil
}

func (s *NovelService) UpdateNovelContent(novelID uint, req *UpdateNovelContentRequest) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"setup_content":   strings.TrimSpace(req.SetupContent),
			"outline_content": strings.TrimSpace(req.OutlineContent),
			"status":          "editing",
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
		}
		if err := tx.Model(&models.Novel{}).Where("id = ?", novelID).Updates(updates).Error; err != nil {
			return err
		}
		for _, chapter := range req.Chapters {
			if chapter.ChapterNumber <= 0 {
				continue
			}
			chapterUpdates := map[string]interface{}{
				"title":      strings.TrimSpace(chapter.Title),
				"outline":    strings.TrimSpace(chapter.Outline),
				"status":     "outlined",
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}
			if err := tx.Model(&models.NovelChapter{}).
				Where("novel_id = ? AND chapter_number = ?", novelID, chapter.ChapterNumber).
				Updates(chapterUpdates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *NovelService) UpdateChapterContent(novelID uint, chapterNumber int, req *UpdateChapterContentRequest) error {
	updateData := map[string]interface{}{
		"draft_content": strings.TrimSpace(req.DraftContent),
		"final_content": strings.TrimSpace(req.FinalContent),
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if strings.TrimSpace(req.FinalContent) != "" {
		updateData["status"] = "finalized"
	} else if strings.TrimSpace(req.DraftContent) != "" {
		updateData["status"] = "draft_ready"
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.NovelChapter{}).
			Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).
			Updates(updateData).Error; err != nil {
			return err
		}
		return tx.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
			"status":          "editing",
			"current_chapter": chapterNumber,
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
	})
}

func (s *NovelService) GenerateSetup(novelID uint, model string) (string, error) {
	task, err := s.taskService.CreateTask("novel_setup_generation", strconv.Itoa(int(novelID)))
	if err != nil {
		return "", err
	}
	go s.processSetup(task.ID, novelID, model)
	return task.ID, nil
}

func (s *NovelService) GenerateOutline(novelID uint, model string) (string, error) {
	task, err := s.taskService.CreateTask("novel_outline_generation", strconv.Itoa(int(novelID)))
	if err != nil {
		return "", err
	}
	go s.processOutline(task.ID, novelID, model)
	return task.ID, nil
}

func (s *NovelService) GenerateChapterDraft(novelID uint, chapterNumber int, model string) (string, error) {
	task, err := s.taskService.CreateTask("novel_chapter_draft_generation", fmt.Sprintf("%d:%d", novelID, chapterNumber))
	if err != nil {
		return "", err
	}
	go s.processDraft(task.ID, novelID, chapterNumber, model)
	return task.ID, nil
}

func (s *NovelService) FinalizeChapter(novelID uint, chapterNumber int, model string) (string, error) {
	task, err := s.taskService.CreateTask("novel_chapter_finalize", fmt.Sprintf("%d:%d", novelID, chapterNumber))
	if err != nil {
		return "", err
	}
	go s.processFinalize(task.ID, novelID, chapterNumber, model)
	return task.ID, nil
}

func (s *NovelService) GenerateAll(novelID uint, model string) (string, error) {
	task, err := s.taskService.CreateTask("novel_full_generation", strconv.Itoa(int(novelID)))
	if err != nil {
		return "", err
	}
	go s.processAll(task.ID, novelID, model)
	return task.ID, nil
}

func (s *NovelService) ApplyToDrama(novelID uint, dramaID uint) error {
	novel, err := s.GetNovel(novelID)
	if err != nil {
		return err
	}
	if novel.DramaID > 0 && novel.DramaID != dramaID {
		return fmt.Errorf("novel and drama are not in same project")
	}
	var drama models.Drama
	if err := s.db.Where("id = ?", dramaID).First(&drama).Error; err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Drama{}).Where("id = ?", dramaID).Updates(map[string]interface{}{
			"title":          novel.Title,
			"genre":          novel.Genre,
			"total_episodes": novel.ChapterCount,
			"status":         "planning",
		}).Error; err != nil {
			return err
		}
		for _, chapter := range novel.Chapters {
			content := ""
			if chapter.FinalContent != nil && strings.TrimSpace(*chapter.FinalContent) != "" {
				content = *chapter.FinalContent
			} else if chapter.DraftContent != nil {
				content = *chapter.DraftContent
			}
			var episode models.Episode
			err := tx.Where("drama_id = ? AND episode_number = ?", dramaID, chapter.ChapterNumber).First(&episode).Error
			title := chapter.Title
			if strings.TrimSpace(title) == "" {
				title = fmt.Sprintf("第%d章", chapter.ChapterNumber)
			}
			if err == nil {
				updateData := map[string]interface{}{
					"title":          title,
					"script_content": content,
					"status":         "draft",
				}
				if chapter.Outline != nil && strings.TrimSpace(*chapter.Outline) != "" {
					updateData["description"] = *chapter.Outline
				}
				if err := tx.Model(&models.Episode{}).Where("id = ?", episode.ID).Updates(updateData).Error; err != nil {
					return err
				}
				continue
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
			newEpisode := models.Episode{
				DramaID:       dramaID,
				EpisodeNum:    chapter.ChapterNumber,
				Title:         title,
				ScriptContent: &content,
				Status:        "draft",
			}
			if chapter.Outline != nil && strings.TrimSpace(*chapter.Outline) != "" {
				newEpisode.Description = chapter.Outline
			}
			if err := tx.Create(&newEpisode).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *NovelService) ExportTXT(novelID uint) (string, string, error) {
	novel, err := s.GetNovel(novelID)
	if err != nil {
		return "", "", err
	}
	var builder strings.Builder
	builder.WriteString(novel.Title)
	builder.WriteString("\n\n")
	builder.WriteString("类型：")
	builder.WriteString(novel.Genre)
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("章节数：%d\n", novel.ChapterCount))
	builder.WriteString(fmt.Sprintf("每章字数：%d\n", novel.WordsPerChapter))
	if strings.TrimSpace(novel.Requirement) != "" {
		builder.WriteString("大概需求：")
		builder.WriteString(strings.TrimSpace(novel.Requirement))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	if novel.SetupContent != nil && strings.TrimSpace(*novel.SetupContent) != "" {
		builder.WriteString("【设定】\n")
		builder.WriteString(strings.TrimSpace(*novel.SetupContent))
		builder.WriteString("\n\n")
	}
	if novel.OutlineContent != nil && strings.TrimSpace(*novel.OutlineContent) != "" {
		builder.WriteString("【目录】\n")
		builder.WriteString(strings.TrimSpace(*novel.OutlineContent))
		builder.WriteString("\n\n")
	}
	for _, ch := range novel.Chapters {
		title := ch.Title
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("第%d章", ch.ChapterNumber)
		}
		builder.WriteString(title)
		builder.WriteString("\n\n")
		content := ""
		if ch.FinalContent != nil && strings.TrimSpace(*ch.FinalContent) != "" {
			content = strings.TrimSpace(*ch.FinalContent)
		} else if ch.DraftContent != nil {
			content = strings.TrimSpace(*ch.DraftContent)
		}
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	fileName := fmt.Sprintf("%s.txt", sanitizeFileName(novel.Title))
	return fileName, builder.String(), nil
}

func (s *NovelService) processSetup(taskID string, novelID uint, model string) {
	if err := s.taskService.UpdateTaskStatus(taskID, "processing", 10, "正在生成小说设定"); err != nil {
		return
	}
	novel, err := s.GetNovel(novelID)
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	prompt := fmt.Sprintf(`你是资深中文小说策划。请根据以下信息输出完整小说设定，要求包含：世界观、核心冲突、主要角色关系、叙事风格、章节推进策略。
小说名：%s
类型：%s
章节数：%d
每章字数：%d
用户需求：%s
输出纯文本，结构清晰。`, novel.Title, novel.Genre, novel.ChapterCount, novel.WordsPerChapter, novel.Requirement)
	text, err := s.generateTextWithModel(model, prompt, "你擅长进行中文长篇小说策划。", ai.WithMaxTokens(3000))
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if err := s.db.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
		"setup_content":   text,
		"status":          "setup_ready",
		"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
		"current_chapter": 0,
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	s.taskService.UpdateTaskResult(taskID, map[string]interface{}{
		"novel_id": novelID,
		"step":     "setup",
	})
}

func (s *NovelService) processOutline(taskID string, novelID uint, model string) {
	if err := s.taskService.UpdateTaskStatus(taskID, "processing", 10, "正在生成章节目录"); err != nil {
		return
	}
	novel, err := s.GetNovel(novelID)
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if novel.SetupContent == nil || strings.TrimSpace(*novel.SetupContent) == "" {
		s.taskService.UpdateTaskError(taskID, fmt.Errorf("请先生成设定"))
		return
	}
	prompt := fmt.Sprintf(`请基于以下小说设定，输出章节目录。必须返回JSON，格式：
{"chapters":[{"chapter_number":1,"title":"章节名","outline":"本章概要"}]}
章节总数必须为%d，标题和概要必须完整。
设定：
%s`, novel.ChapterCount, strings.TrimSpace(*novel.SetupContent))
	text, err := s.generateTextWithModel(model, prompt, "你擅长进行中文小说大纲拆分。", ai.WithMaxTokens(6000))
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	var parsed struct {
		Chapters []ChapterOutlineItem `json:"chapters"`
	}
	if err := utils.SafeParseAIJSON(text, &parsed); err != nil || len(parsed.Chapters) == 0 {
		s.taskService.UpdateTaskError(taskID, fmt.Errorf("目录解析失败"))
		return
	}
	if len(parsed.Chapters) > novel.ChapterCount {
		parsed.Chapters = parsed.Chapters[:novel.ChapterCount]
	}
	for i := range parsed.Chapters {
		if parsed.Chapters[i].ChapterNumber <= 0 {
			parsed.Chapters[i].ChapterNumber = i + 1
		}
	}
	outlineText := make([]string, 0, len(parsed.Chapters))
	err = s.db.Transaction(func(tx *gorm.DB) error {
		for _, chapter := range parsed.Chapters {
			title := strings.TrimSpace(chapter.Title)
			if title == "" {
				title = fmt.Sprintf("第%d章", chapter.ChapterNumber)
			}
			outline := strings.TrimSpace(chapter.Outline)
			outlineText = append(outlineText, fmt.Sprintf("%d. %s\n%s", chapter.ChapterNumber, title, outline))
			updateData := map[string]interface{}{
				"title":      title,
				"outline":    outline,
				"status":     "outlined",
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}
			if err := tx.Model(&models.NovelChapter{}).
				Where("novel_id = ? AND chapter_number = ?", novelID, chapter.ChapterNumber).
				Updates(updateData).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
			"outline_content": strings.Join(outlineText, "\n\n"),
			"status":          "outline_ready",
			"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
	})
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	s.taskService.UpdateTaskResult(taskID, map[string]interface{}{
		"novel_id": novelID,
		"step":     "outline",
	})
}

func (s *NovelService) processDraft(taskID string, novelID uint, chapterNumber int, model string) {
	if err := s.taskService.UpdateTaskStatus(taskID, "processing", 10, "正在生成章节草稿"); err != nil {
		return
	}
	novel, err := s.GetNovel(novelID)
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	var chapter models.NovelChapter
	if err := s.db.Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).First(&chapter).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	setup := ""
	if novel.SetupContent != nil {
		setup = *novel.SetupContent
	}
	outline := ""
	if chapter.Outline != nil {
		outline = *chapter.Outline
	}
	prompt := fmt.Sprintf(`请生成中文小说第%d章草稿。
小说名：%s
类型：%s
目标字数：约%d字
全局设定：
%s
本章标题：%s
本章概要：%s
要求：有叙事层次、对话自然、场景清晰，输出纯正文。`, chapterNumber, novel.Title, novel.Genre, novel.WordsPerChapter, strings.TrimSpace(setup), chapter.Title, strings.TrimSpace(outline))
	text, err := s.generateTextWithModel(model, prompt, "你擅长写中文网络小说正文。", ai.WithMaxTokens(8000))
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if err := s.db.Model(&models.NovelChapter{}).Where("id = ?", chapter.ID).Updates(map[string]interface{}{
		"draft_content": text,
		"status":        "draft_ready",
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if err := s.db.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
		"status":          "draft_ready",
		"current_chapter": chapterNumber,
		"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	s.taskService.UpdateTaskResult(taskID, map[string]interface{}{
		"novel_id":       novelID,
		"chapter_number": chapterNumber,
		"step":           "draft",
	})
}

func (s *NovelService) processFinalize(taskID string, novelID uint, chapterNumber int, model string) {
	if err := s.taskService.UpdateTaskStatus(taskID, "processing", 10, "正在定稿章节"); err != nil {
		return
	}
	var chapter models.NovelChapter
	if err := s.db.Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).First(&chapter).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	draft := ""
	if chapter.DraftContent != nil {
		draft = *chapter.DraftContent
	}
	if strings.TrimSpace(draft) == "" {
		s.taskService.UpdateTaskError(taskID, fmt.Errorf("请先生成草稿"))
		return
	}
	prompt := fmt.Sprintf(`请对以下章节草稿进行定稿润色，要求：
1. 保持剧情一致
2. 提升语言质量和节奏
3. 消除重复表达和逻辑断裂
4. 输出纯正文
章节标题：%s
草稿内容：
%s`, chapter.Title, draft)
	text, err := s.generateTextWithModel(model, prompt, "你擅长中文小说编辑定稿。", ai.WithMaxTokens(8000))
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if err := s.db.Model(&models.NovelChapter{}).Where("id = ?", chapter.ID).Updates(map[string]interface{}{
		"final_content": text,
		"status":        "finalized",
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if err := s.db.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
		"status":          "finalizing",
		"current_chapter": chapterNumber,
		"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	s.taskService.UpdateTaskResult(taskID, map[string]interface{}{
		"novel_id":       novelID,
		"chapter_number": chapterNumber,
		"step":           "finalize",
	})
}

func (s *NovelService) processAll(taskID string, novelID uint, model string) {
	if err := s.taskService.UpdateTaskStatus(taskID, "processing", 5, "开始生成小说全流程"); err != nil {
		return
	}
	novel, err := s.GetNovel(novelID)
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	if novel.SetupContent == nil || strings.TrimSpace(*novel.SetupContent) == "" {
		setupTaskID := taskID + "_setup"
		s.processSetup(setupTaskID, novelID, model)
	}
	if novel.OutlineContent == nil || strings.TrimSpace(*novel.OutlineContent) == "" {
		outlineTaskID := taskID + "_outline"
		s.processOutline(outlineTaskID, novelID, model)
	}
	novel, err = s.GetNovel(novelID)
	if err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	total := len(novel.Chapters)
	if total == 0 {
		s.taskService.UpdateTaskError(taskID, fmt.Errorf("章节不存在"))
		return
	}
	for idx, chapter := range novel.Chapters {
		startProgress := 20 + int(float64(idx)/float64(total)*70)
		s.taskService.UpdateTaskStatus(taskID, "processing", startProgress, fmt.Sprintf("正在生成第%d章草稿", chapter.ChapterNumber))
		if chapter.DraftContent == nil || strings.TrimSpace(*chapter.DraftContent) == "" {
			s.processDraft(taskID+"_draft_"+strconv.Itoa(chapter.ChapterNumber), novelID, chapter.ChapterNumber, model)
		}
		s.taskService.UpdateTaskStatus(taskID, "processing", startProgress+5, fmt.Sprintf("正在定稿第%d章", chapter.ChapterNumber))
		var refreshed models.NovelChapter
		if err := s.db.Where("id = ?", chapter.ID).First(&refreshed).Error; err != nil {
			s.taskService.UpdateTaskError(taskID, err)
			return
		}
		if refreshed.FinalContent == nil || strings.TrimSpace(*refreshed.FinalContent) == "" {
			s.processFinalize(taskID+"_final_"+strconv.Itoa(chapter.ChapterNumber), novelID, chapter.ChapterNumber, model)
		}
	}
	if err := s.db.Model(&models.Novel{}).Where("id = ?", novelID).Updates(map[string]interface{}{
		"status":          "completed",
		"current_chapter": total,
		"updated_at":      gorm.Expr("CURRENT_TIMESTAMP"),
	}).Error; err != nil {
		s.taskService.UpdateTaskError(taskID, err)
		return
	}
	s.taskService.UpdateTaskResult(taskID, map[string]interface{}{
		"novel_id":       novelID,
		"completed_step": "all",
		"chapter_count":  total,
	})
}

func (s *NovelService) generateTextWithModel(model string, prompt string, systemPrompt string, options ...func(*ai.ChatCompletionRequest)) (string, error) {
	if model == "" {
		return s.aiService.GenerateText(prompt, systemPrompt, options...)
	}
	client, err := s.aiService.GetAIClientForModel("text", model)
	if err != nil {
		return s.aiService.GenerateText(prompt, systemPrompt, options...)
	}
	return client.GenerateText(prompt, systemPrompt, options...)
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	result := strings.TrimSpace(replacer.Replace(name))
	if result == "" {
		return "novel"
	}
	return result
}
