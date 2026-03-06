import request from '../utils/request'
import type { ChapterEditInput, CreateNovelRequest, Novel, NovelListQuery } from '../types/novel'

export interface NovelTaskResponse {
  task_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  message: string
}

export const novelAPI = {
  list(params?: NovelListQuery) {
    return request.get<{
      items: Novel[]
      pagination: {
        page: number
        page_size: number
        total: number
        total_pages: number
      }
    }>('/novels', { params })
  },

  create(data: CreateNovelRequest) {
    return request.post<Novel>('/novels', data)
  },

  get(id: string | number) {
    return request.get<Novel>(`/novels/${id}`)
  },

  updateContent(id: string | number, data: { setup_content: string; outline_content: string; chapters: ChapterEditInput[] }) {
    return request.put<Novel>(`/novels/${id}/content`, data)
  },

  updateChapterContent(id: string | number, chapterNumber: number, data: { draft_content: string; final_content: string }) {
    return request.put<Novel>(`/novels/${id}/chapters/${chapterNumber}/content`, data)
  },

  generateSetup(id: string | number, model = '') {
    return request.post<NovelTaskResponse>(`/novels/${id}/generate/setup`, { model })
  },

  generateOutline(id: string | number, model = '') {
    return request.post<NovelTaskResponse>(`/novels/${id}/generate/outline`, { model })
  },

  generateDraft(id: string | number, chapterNumber: number, model = '') {
    return request.post<NovelTaskResponse>(`/novels/${id}/chapters/${chapterNumber}/generate-draft`, { model })
  },

  finalizeChapter(id: string | number, chapterNumber: number, model = '') {
    return request.post<NovelTaskResponse>(`/novels/${id}/chapters/${chapterNumber}/finalize`, { model })
  },

  generateAll(id: string | number, model = '') {
    return request.post<NovelTaskResponse>(`/novels/${id}/generate/all`, { model })
  },

  applyToDrama(id: string | number, dramaId: string | number) {
    return request.post<{ message: string }>(`/novels/${id}/apply/dramas/${dramaId}`)
  },
}
