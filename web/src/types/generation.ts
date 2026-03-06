import type { ID } from './drama'

export interface GenerateCharactersRequest {
  drama_id: ID
  episode_id?: ID
  outline?: string
  count?: number
  temperature?: number
  model?: string  // 指定使用的文本模型
}

export interface ParseScriptRequest {
  drama_id: ID
  script_content: string
  auto_split?: boolean
}

export interface ParseScriptResult {
  episodes: ParsedEpisode[]
  characters: ParsedCharacter[]
  summary: string
}

export interface ParsedCharacter {
  name: string
  role: string
  description: string
  personality: string
}

export interface ParsedEpisode {
  id?: ID
  episode_number: number
  title: string
  description: string
  script_content: string
  duration: number
  scenes?: any[]
  chapter_start?: number
  chapter_end?: number
  start_marker?: string
  end_marker?: string
}
