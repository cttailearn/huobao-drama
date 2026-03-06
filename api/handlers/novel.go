package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/drama-generator/backend/application/services"
	"github.com/drama-generator/backend/pkg/config"
	"github.com/drama-generator/backend/pkg/logger"
	"github.com/drama-generator/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NovelHandler struct {
	novelService *services.NovelService
	log          *logger.Logger
}

func NewNovelHandler(db *gorm.DB, cfg *config.Config, log *logger.Logger) *NovelHandler {
	return &NovelHandler{
		novelService: services.NewNovelService(db, cfg, log),
		log:          log,
	}
}

func (h *NovelHandler) CreateNovel(c *gin.Context) {
	var req services.CreateNovelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	novel, err := h.novelService.CreateNovel(&req)
	if err != nil {
		h.log.Errorw("Failed to create novel", "error", err)
		response.InternalError(c, err.Error())
		return
	}
	response.Created(c, novel)
}

func (h *NovelHandler) ListNovels(c *gin.Context) {
	var query services.ListNovelQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	items, total, err := h.novelService.ListNovels(&query)
	if err != nil {
		h.log.Errorw("Failed to list novels", "error", err)
		response.InternalError(c, err.Error())
		return
	}
	response.SuccessWithPagination(c, items, total, query.Page, query.PageSize)
}

func (h *NovelHandler) GetNovel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return
	}
	novel, err := h.novelService.GetNovel(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "小说不存在")
			return
		}
		h.log.Errorw("Failed to get novel", "error", err, "novel_id", id)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, novel)
}

func (h *NovelHandler) UpdateNovelContent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return
	}
	var req services.UpdateNovelContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.novelService.UpdateNovelContent(uint(id), &req); err != nil {
		h.log.Errorw("Failed to update novel content", "error", err, "novel_id", id)
		response.InternalError(c, err.Error())
		return
	}
	novel, err := h.novelService.GetNovel(uint(id))
	if err != nil {
		h.log.Errorw("Failed to fetch updated novel", "error", err, "novel_id", id)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, novel)
}

func (h *NovelHandler) UpdateChapterContent(c *gin.Context) {
	novelID, chapterNumber, _, ok := h.parseChapterParams(c)
	if !ok {
		return
	}
	var req services.UpdateChapterContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.novelService.UpdateChapterContent(novelID, chapterNumber, &req); err != nil {
		h.log.Errorw("Failed to update chapter content", "error", err, "novel_id", novelID, "chapter", chapterNumber)
		response.InternalError(c, err.Error())
		return
	}
	novel, err := h.novelService.GetNovel(novelID)
	if err != nil {
		h.log.Errorw("Failed to fetch updated novel", "error", err, "novel_id", novelID)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, novel)
}

func (h *NovelHandler) GenerateSetup(c *gin.Context) {
	h.runGeneration(c, func(novelID uint, model string) (string, error) {
		return h.novelService.GenerateSetup(novelID, model)
	}, "设定生成任务已创建")
}

func (h *NovelHandler) GenerateOutline(c *gin.Context) {
	h.runGeneration(c, func(novelID uint, model string) (string, error) {
		return h.novelService.GenerateOutline(novelID, model)
	}, "目录生成任务已创建")
}

func (h *NovelHandler) GenerateDraft(c *gin.Context) {
	novelID, chapterNumber, model, ok := h.parseChapterParams(c)
	if !ok {
		return
	}
	taskID, err := h.novelService.GenerateChapterDraft(novelID, chapterNumber, model)
	if err != nil {
		h.log.Errorw("Failed to generate chapter draft", "error", err, "novel_id", novelID, "chapter", chapterNumber)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"task_id": taskID,
		"status":  "pending",
		"message": "章节草稿任务已创建",
	})
}

func (h *NovelHandler) FinalizeChapter(c *gin.Context) {
	novelID, chapterNumber, model, ok := h.parseChapterParams(c)
	if !ok {
		return
	}
	taskID, err := h.novelService.FinalizeChapter(novelID, chapterNumber, model)
	if err != nil {
		h.log.Errorw("Failed to finalize chapter", "error", err, "novel_id", novelID, "chapter", chapterNumber)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"task_id": taskID,
		"status":  "pending",
		"message": "章节定稿任务已创建",
	})
}

func (h *NovelHandler) GenerateAll(c *gin.Context) {
	h.runGeneration(c, func(novelID uint, model string) (string, error) {
		return h.novelService.GenerateAll(novelID, model)
	}, "全流程生成任务已创建")
}

func (h *NovelHandler) ApplyToDrama(c *gin.Context) {
	novelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return
	}
	dramaID, err := strconv.ParseUint(c.Param("drama_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid drama id")
		return
	}
	if err := h.novelService.ApplyToDrama(uint(novelID), uint(dramaID)); err != nil {
		h.log.Errorw("Failed to apply novel to drama", "error", err, "novel_id", novelID, "drama_id", dramaID)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message": "已应用到短剧",
	})
}

func (h *NovelHandler) ExportTXT(c *gin.Context) {
	novelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return
	}
	fileName, content, err := h.novelService.ExportTXT(uint(novelID))
	if err != nil {
		h.log.Errorw("Failed to export novel txt", "error", err, "novel_id", novelID)
		response.InternalError(c, err.Error())
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	encodedFileName := url.QueryEscape(fileName)
	encodedFileName = strings.ReplaceAll(encodedFileName, "+", "%20")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="novel.txt"; filename*=UTF-8''%s`, encodedFileName))
	c.String(http.StatusOK, content)
}

func (h *NovelHandler) runGeneration(c *gin.Context, runner func(uint, string) (string, error), message string) {
	novelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	_ = c.ShouldBindJSON(&req)
	taskID, err := runner(uint(novelID), req.Model)
	if err != nil {
		h.log.Errorw("Failed to run novel generation", "error", err, "novel_id", novelID)
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"task_id": taskID,
		"status":  "pending",
		"message": message,
	})
}

func (h *NovelHandler) parseChapterParams(c *gin.Context) (uint, int, string, bool) {
	novelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid novel id")
		return 0, 0, "", false
	}
	chapterNumber, err := strconv.Atoi(c.Param("chapter_number"))
	if err != nil || chapterNumber <= 0 {
		response.BadRequest(c, "invalid chapter number")
		return 0, 0, "", false
	}
	var req struct {
		Model string `json:"model"`
	}
	_ = c.ShouldBindJSON(&req)
	return uint(novelID), chapterNumber, req.Model, true
}
