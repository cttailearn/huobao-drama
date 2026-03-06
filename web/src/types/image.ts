import type { ID } from './drama'

export interface ImageGeneration {
  id: number
  storyboard_id?: ID
  scene_id?: ID
  drama_id: ID
  character_id?: ID
  image_type?: string
  frame_type?: string
  provider: string
  prompt: string
  negative_prompt?: string
  model?: string
  size?: string
  quality?: string
  style?: string
  steps?: number
  cfg_scale?: number
  seed?: number
  image_url?: string
  image_generation?: any
  local_path?: string
  status: ImageStatus
  task_id?: string
  error_msg?: string
  width?: number
  height?: number
  created_at: string
  updated_at: string
  completed_at?: string
}

export type ImageStatus = 'pending' | 'processing' | 'completed' | 'failed'

export type ImageProvider = 'openai' | 'dalle' | 'midjourney' | 'stable_diffusion' | 'sd'

export interface GenerateImageRequest {
  scene_id?: ID
  storyboard_id?: ID
  drama_id: ID
  image_type?: string
  frame_type?: string
  prompt: string
  negative_prompt?: string
  reference_images?: string[]
  provider?: string
  model?: string
  size?: string
  quality?: string
  style?: string
  steps?: number
  cfg_scale?: number
  seed?: number
  width?: number
  height?: number
}

export interface ImageGenerationListParams {
  drama_id?: ID
  scene_id?: ID
  storyboard_id?: ID
  frame_type?: string
  status?: ImageStatus
  page?: number
  page_size?: number
}
