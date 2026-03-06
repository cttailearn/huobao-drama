export interface NovelChapter {
  id: number
  novel_id: number
  chapter_number: number
  title: string
  outline?: string
  draft_content?: string
  final_content?: string
  status: string
  created_at: string
  updated_at: string
}

export interface Novel {
  id: number
  drama_id: number
  title: string
  genre: string
  chapter_count: number
  words_per_chapter: number
  requirement?: string
  status: string
  setup_content?: string
  outline_content?: string
  current_chapter: number
  created_at: string
  updated_at: string
  chapters: NovelChapter[]
}

export interface CreateNovelRequest {
  drama_id: number
  title: string
  genre: string
  chapter_count: number
  words_per_chapter: number
  requirement?: string
}

export interface NovelListQuery {
  page?: number
  page_size?: number
  drama_id?: number
}

export interface ChapterEditInput {
  chapter_number: number
  title: string
  outline: string
}
