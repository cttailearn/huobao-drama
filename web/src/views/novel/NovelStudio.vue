<template>
  <div class="page-container">
    <div class="content-wrapper animate-fade-in">
      <AppHeader :fixed="false">
        <template #left>
          <el-button text class="back-btn" @click="handleBackToProject">
            <el-icon><ArrowLeft /></el-icon>
            <span>返回项目</span>
          </el-button>
          <div class="page-title">
            <h1>AI小说生成</h1>
            <span class="subtitle">设定→目录→具体内容→修改保存</span>
          </div>
        </template>
      </AppHeader>

      <div class="novel-layout">
        <el-card class="left-card">
          <template #header>
            <div class="card-header">新建小说</div>
          </template>
          <el-form :model="createForm" label-position="top">
            <el-form-item label="所属项目">
              <div class="project-select-row">
                <el-select v-model="selectedDramaId" :disabled="isProjectScoped" placeholder="请选择项目" style="flex: 1">
                  <el-option v-for="d in dramas" :key="d.id" :label="d.title" :value="Number(d.id)" />
                </el-select>
                <el-button v-if="!isProjectScoped" @click="handleCreateProject">新建项目</el-button>
              </div>
            </el-form-item>
            <el-form-item label="小说名">
              <el-input v-model="createForm.title" placeholder="请输入小说名" />
            </el-form-item>
            <el-form-item label="类型">
              <el-input v-model="createForm.genre" placeholder="如：都市、玄幻、悬疑" />
            </el-form-item>
            <el-form-item label="章节数">
              <el-input-number v-model="createForm.chapter_count" :min="1" :max="200" style="width: 100%" />
            </el-form-item>
            <el-form-item label="每章字数">
              <el-input-number v-model="createForm.words_per_chapter" :min="200" :max="12000" :step="100" style="width: 100%" />
            </el-form-item>
            <el-form-item label="大概需求">
              <el-input
                v-model="createForm.requirement"
                type="textarea"
                :rows="4"
                placeholder="例如：希望女主成长线明显，都市悬疑反转风格，结局治愈"
              />
            </el-form-item>
            <el-button type="primary" style="width: 100%" :loading="creating" @click="handleCreateNovel">创建小说</el-button>
          </el-form>

          <div class="list-header">
            <span>小说列表（当前项目）</span>
            <el-button text @click="loadNovels">刷新</el-button>
          </div>
          <el-scrollbar height="420px">
            <div
              v-for="item in novels"
              :key="item.id"
              class="novel-item"
              :class="{ active: selectedNovelId === item.id }"
              @click="selectNovel(item.id)"
            >
              <div class="title">{{ item.title }}</div>
              <div class="meta">{{ item.genre }} · {{ item.chapter_count }}章 · {{ item.status }}</div>
            </div>
          </el-scrollbar>
        </el-card>

        <el-card v-loading="detailLoading" class="right-card">
          <template #header>
            <div class="card-header">
              <span>生成控制台</span>
              <div class="header-actions">
                <span class="project-label">当前项目：{{ currentDramaTitle || '未选择' }}</span>
                <el-button type="success" :disabled="!selectedDramaId || !novelDetail" @click="handleApplyToDrama">应用到短剧</el-button>
                <el-button :disabled="!novelDetail" @click="downloadTxt">导出TXT</el-button>
              </div>
            </div>
          </template>

          <template v-if="novelDetail">
            <div class="step-buttons">
              <el-button type="primary" :loading="actionLoading === 'setup'" @click="handleGenerateSetup">
                {{ editableSetup.trim() ? '重新生成设定' : '第一步 生成设定' }}
              </el-button>
              <el-button type="primary" :loading="actionLoading === 'outline'" @click="handleGenerateOutline">
                {{ editableOutline.trim() ? '重新生成目录' : '第二步 生成目录' }}
              </el-button>
              <el-select v-model="selectedChapterNumber" style="width: 130px">
                <el-option
                  v-for="chapter in editableChapters"
                  :key="chapter.chapter_number"
                  :label="`第${chapter.chapter_number}章`"
                  :value="chapter.chapter_number"
                />
              </el-select>
              <el-button type="warning" :loading="actionLoading === 'draft'" @click="handleGenerateDraft">第三步 生成具体内容</el-button>
              <el-button type="success" :loading="saveLoading || chapterSaveLoading" @click="handleStep4Save">第四步 修改保存</el-button>
            </div>

            <div v-if="currentTask" class="task-box">
              <div class="task-title">模型进度：{{ currentTask.status }}</div>
              <el-progress :percentage="currentTask.progress" />
              <div class="task-msg">{{ currentTask.message || currentTask.error || '处理中...' }}</div>
            </div>

            <el-row :gutter="16" class="content-grid">
              <el-col :span="12">
                <el-card>
                  <template #header>
                    <div class="section-header">
                      <span>设定</span>
                      <span class="section-tip">可直接编辑后保存</span>
                    </div>
                  </template>
                  <el-input
                    v-model="editableSetup"
                    type="textarea"
                    :rows="14"
                  />
                </el-card>
              </el-col>
              <el-col :span="12">
                <el-card class="chapter-editor-side">
                  <template #header>
                    <div class="section-header">
                      <span>章节内容编辑（第{{ selectedChapterNumber }}章）</span>
                      <el-button :loading="chapterSaveLoading" @click="handleSaveChapterContent">保存当前内容</el-button>
                    </div>
                  </template>
                  <el-input v-model="chapterContent" type="textarea" :rows="14" />
                </el-card>
              </el-col>
            </el-row>

            <el-table :data="editableChapters" style="width: 100%; margin-top: 16px">
              <el-table-column prop="chapter_number" label="章" width="60" />
              <el-table-column label="标题" width="220">
                <template #default="{ row }">
                  <el-input v-model="row.title" />
                </template>
              </el-table-column>
              <el-table-column label="章节概要" min-width="320">
                <template #default="{ row }">
                  <el-input v-model="row.outline" type="textarea" :rows="2" />
                </template>
              </el-table-column>
              <el-table-column prop="status" label="状态" width="120" />
            </el-table>
          </template>
          <el-empty v-else description="请选择或创建小说" />
        </el-card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { AppHeader } from '@/components/common'
import { novelAPI } from '@/api/novel'
import { dramaAPI } from '@/api/drama'
import { taskAPI, type AsyncTask } from '@/api/task'
import type { Drama } from '@/types/drama'
import type { ChapterEditInput, CreateNovelRequest, Novel, NovelChapter } from '@/types/novel'

const router = useRouter()
const route = useRoute()
const novels = ref<Novel[]>([])
const novelDetail = ref<Novel | null>(null)
const selectedNovelId = ref<number | null>(null)
const selectedChapterNumber = ref(1)
const selectedDramaId = ref<number | null>(null)
const currentTask = ref<AsyncTask | null>(null)
const creating = ref(false)
const detailLoading = ref(false)
const actionLoading = ref('')
const dramas = ref<Drama[]>([])
const pollTimer = ref<number | null>(null)
const saveLoading = ref(false)
const editableSetup = ref('')
const editableOutline = ref('')
const chapterSaveLoading = ref(false)
const chapterContent = ref('')

type EditableChapter = ChapterEditInput & Pick<NovelChapter, 'status'> & Partial<Pick<NovelChapter, 'draft_content' | 'final_content'>>
const editableChapters = ref<EditableChapter[]>([])

const createForm = ref<CreateNovelRequest>({
  drama_id: 0,
  title: '',
  genre: '',
  chapter_count: 12,
  words_per_chapter: 1500,
  requirement: '',
})

const scopedDramaId = computed(() => {
  const fromParam = Number(route.params.id || 0)
  if (fromParam > 0) return fromParam
  const fromQuery = Number(route.query.drama_id || 0)
  if (fromQuery > 0) return fromQuery
  return 0
})

const isProjectScoped = computed(() => scopedDramaId.value > 0)

const currentDramaTitle = computed(() => {
  const item = dramas.value.find((d) => Number(d.id) === selectedDramaId.value)
  return item?.title || ''
})

const getTaskStorageKey = (novelId: number) => `novel_task_${novelId}`

const clearStoredTask = () => {
  if (!selectedNovelId.value) return
  localStorage.removeItem(getTaskStorageKey(selectedNovelId.value))
}

const clearPolling = () => {
  if (pollTimer.value) {
    window.clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

const startPollingTask = async (taskId: string, action: string) => {
  clearPolling()
  if (selectedNovelId.value) {
    localStorage.setItem(getTaskStorageKey(selectedNovelId.value), JSON.stringify({ taskId, action }))
  }
  try {
    currentTask.value = await taskAPI.getStatus(taskId)
  } catch {}
  pollTimer.value = window.setInterval(async () => {
    try {
      const status = await taskAPI.getStatus(taskId)
      currentTask.value = status
      if (status.status === 'completed' || status.status === 'failed') {
        clearPolling()
        clearStoredTask()
        await loadNovelDetail()
        actionLoading.value = ''
      }
    } catch (error: any) {
      clearPolling()
      actionLoading.value = ''
      ElMessage.error(error.message || '任务状态查询失败')
    }
  }, 2000)
}

const loadNovels = async () => {
  if (!selectedDramaId.value) {
    novels.value = []
    return
  }
  const res = await novelAPI.list({ page: 1, page_size: 100, drama_id: selectedDramaId.value })
  novels.value = res.items || []
}

const loadDramas = async () => {
  if (isProjectScoped.value) {
    const data = await dramaAPI.get(scopedDramaId.value)
    dramas.value = [data]
    selectedDramaId.value = Number(data.id)
  } else {
    const res = await dramaAPI.list({ page: 1, page_size: 200 })
    dramas.value = res.items || []
    if (!selectedDramaId.value && dramas.value.length > 0) {
      selectedDramaId.value = Number(dramas.value[0].id)
    }
  }
  createForm.value.drama_id = selectedDramaId.value || 0
}

const loadNovelDetail = async () => {
  if (!selectedNovelId.value) return
  detailLoading.value = true
  try {
    const data = await novelAPI.get(selectedNovelId.value)
    novelDetail.value = data
    selectedDramaId.value = data.drama_id
    createForm.value.drama_id = data.drama_id
    editableSetup.value = data.setup_content || ''
    editableOutline.value = data.outline_content || ''
    editableChapters.value = data.chapters.map((item) => ({
      chapter_number: item.chapter_number,
      title: item.title || `第${item.chapter_number}章`,
      outline: item.outline || '',
      status: item.status,
      draft_content: item.draft_content,
      final_content: item.final_content,
    }))
    if (data.chapters.length > 0) {
      selectedChapterNumber.value = data.current_chapter > 0 ? data.current_chapter : data.chapters[0].chapter_number
    }
    syncChapterContentEditor()
  } finally {
    detailLoading.value = false
  }
}

const selectNovel = async (id: number) => {
  selectedNovelId.value = id
  currentTask.value = null
  await loadNovelDetail()
  const storedTask = localStorage.getItem(getTaskStorageKey(id))
  if (storedTask) {
    try {
      const parsed = JSON.parse(storedTask) as { taskId: string; action: string }
      actionLoading.value = parsed.action || ''
      await startPollingTask(parsed.taskId, parsed.action || '')
    } catch {
      localStorage.removeItem(getTaskStorageKey(id))
    }
  }
}

const handleCreateNovel = async () => {
  if (!selectedDramaId.value) {
    ElMessage.warning('请先选择或创建项目')
    return
  }
  if (!createForm.value.title.trim() || !createForm.value.genre.trim()) {
    ElMessage.warning('请填写小说名和类型')
    return
  }
  createForm.value.drama_id = selectedDramaId.value
  creating.value = true
  try {
    const novel = await novelAPI.create(createForm.value)
    ElMessage.success('小说创建成功')
    await loadNovels()
    await selectNovel(novel.id)
  } catch (error: any) {
    ElMessage.error(error.message || '创建失败')
  } finally {
    creating.value = false
  }
}

const handleCreateProject = async () => {
  try {
    const { value } = await ElMessageBox.prompt('请输入项目名称', '新建项目', {
      inputPlaceholder: '例如：都市悬疑剧项目',
      confirmButtonText: '创建',
      cancelButtonText: '取消',
      inputValidator: (v) => !!String(v || '').trim(),
      inputErrorMessage: '项目名称不能为空',
    })
    const drama = await dramaAPI.create({
      title: String(value).trim(),
      style: '写实影视',
    })
    await loadDramas()
    selectedDramaId.value = Number(drama.id)
    createForm.value.drama_id = Number(drama.id)
    await loadNovels()
    ElMessage.success('项目创建成功')
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error.message || '创建项目失败')
    }
  }
}

const handleSaveEdits = async (silent = false) => {
  if (!selectedNovelId.value) return
  saveLoading.value = true
  try {
    const data = await novelAPI.updateContent(selectedNovelId.value, {
      setup_content: editableSetup.value,
      outline_content: editableChapters.value
        .map((item) => `${item.chapter_number}. ${item.title}\n${item.outline || ''}`)
        .join('\n\n'),
      chapters: editableChapters.value,
    })
    novelDetail.value = data
    await loadNovelDetail()
    if (!silent) {
      ElMessage.success('设定与目录已保存')
    }
  } catch (error: any) {
    if (!silent) {
      ElMessage.error(error.message || '保存失败')
    }
    throw error
  } finally {
    saveLoading.value = false
  }
}

const handleBackToProject = async () => {
  if (actionLoading.value && (!currentTask.value || currentTask.value.status === 'pending')) {
    ElMessage.warning('任务创建中，请稍候后再返回，避免创建请求被中断')
    return
  }
  if (selectedNovelId.value && currentTask.value && (currentTask.value.status === 'pending' || currentTask.value.status === 'processing')) {
    localStorage.setItem(
      getTaskStorageKey(selectedNovelId.value),
      JSON.stringify({ taskId: currentTask.value.id, action: actionLoading.value || 'processing' })
    )
    ElMessage.success('任务已在后台继续，稍后可回到小说页查看进度')
  }
  if (selectedDramaId.value) {
    await router.push(`/dramas/${selectedDramaId.value}`)
    return
  }
  await router.push('/')
}

const syncChapterContentEditor = () => {
  const selected = editableChapters.value.find((item) => item.chapter_number === selectedChapterNumber.value)
  chapterContent.value = selected?.final_content || selected?.draft_content || ''
}

const handleSaveChapterContent = async (silent = false) => {
  if (!selectedNovelId.value) return
  chapterSaveLoading.value = true
  const selected = editableChapters.value.find((item) => item.chapter_number === selectedChapterNumber.value)
  try {
    const data = await novelAPI.updateChapterContent(selectedNovelId.value, selectedChapterNumber.value, {
      draft_content: chapterContent.value,
      final_content: chapterContent.value,
    })
    novelDetail.value = data
    await loadNovelDetail()
    if (!silent) {
      ElMessage.success(`第${selectedChapterNumber.value}章内容已保存`)
    }
  } catch (error: any) {
    if (!silent) {
      ElMessage.error(error.message || '章节内容保存失败')
    }
    throw error
  } finally {
    chapterSaveLoading.value = false
  }
}

const handleStep4Save = async () => {
  try {
    await handleSaveEdits(true)
    await handleSaveChapterContent(true)
    ElMessage.success('修改内容已保存')
  } catch (error: any) {
    ElMessage.error(error.message || '保存失败')
  }
}

const handleGenerateSetup = async () => {
  if (!selectedNovelId.value) return
  actionLoading.value = 'setup'
  try {
    const res = await novelAPI.generateSetup(selectedNovelId.value)
    await startPollingTask(res.task_id, 'setup')
  } catch (error: any) {
    actionLoading.value = ''
    ElMessage.error(error.message || '生成设定失败')
  }
}

const handleGenerateOutline = async () => {
  if (!selectedNovelId.value) return
  actionLoading.value = 'outline'
  try {
    const res = await novelAPI.generateOutline(selectedNovelId.value)
    await startPollingTask(res.task_id, 'outline')
  } catch (error: any) {
    actionLoading.value = ''
    ElMessage.error(error.message || '生成目录失败')
  }
}

const handleGenerateDraft = async () => {
  if (!selectedNovelId.value) return
  actionLoading.value = 'draft'
  try {
    const res = await novelAPI.generateDraft(selectedNovelId.value, selectedChapterNumber.value)
    await startPollingTask(res.task_id, 'draft')
  } catch (error: any) {
    actionLoading.value = ''
    ElMessage.error(error.message || '生成草稿失败')
  }
}

const handleApplyToDrama = async () => {
  if (!selectedNovelId.value || !selectedDramaId.value) return
  try {
    await novelAPI.applyToDrama(selectedNovelId.value, selectedDramaId.value)
    ElMessage.success('已应用到短剧，可直接进入章节制作')
  } catch (error: any) {
    ElMessage.error(error.message || '应用失败')
  }
}

const downloadTxt = async () => {
  if (!selectedNovelId.value) return
  try {
    const res = await fetch(`/api/v1/novels/${selectedNovelId.value}/export/txt`)
    if (!res.ok) throw new Error('下载失败')
    const blob = await res.blob()
    const link = document.createElement('a')
    const url = URL.createObjectURL(blob)
    const disposition = res.headers.get('Content-Disposition') || ''
    const match = disposition.match(/filename="(.+)"/)
    link.href = url
    link.download = match?.[1] || 'novel.txt'
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  } catch (error: any) {
    ElMessage.error(error.message || '导出失败')
  }
}

onMounted(async () => {
  await loadDramas()
  await loadNovels()
  if (novels.value.length > 0) {
    await selectNovel(novels.value[0].id)
  }
})

watch(selectedChapterNumber, () => {
  syncChapterContentEditor()
})

watch(selectedDramaId, async (newVal, oldVal) => {
  if (!newVal || newVal === oldVal) return
  if (isProjectScoped.value && newVal !== scopedDramaId.value) {
    selectedDramaId.value = scopedDramaId.value
    return
  }
  createForm.value.drama_id = newVal
  selectedNovelId.value = null
  novelDetail.value = null
  await loadNovels()
  if (novels.value.length > 0) {
    await selectNovel(novels.value[0].id)
  }
})

onUnmounted(() => {
  clearPolling()
})
</script>

<style scoped>
.novel-layout {
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: 16px;
  margin-top: 16px;
}

.left-card,
.right-card {
  min-height: calc(100vh - 140px);
}

.card-header {
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.list-header {
  margin-top: 16px;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.novel-item {
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  padding: 10px;
  margin-bottom: 8px;
  cursor: pointer;
}

.novel-item.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.novel-item .title {
  font-weight: 600;
  margin-bottom: 4px;
}

.novel-item .meta {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.header-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.project-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.project-select-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.step-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.task-box {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 12px;
}

.task-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.task-msg {
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.content-grid {
  margin-top: 12px;
}

.chapter-editor-side {
  height: 100%;
}

@media (max-width: 1200px) {
  .novel-layout {
    grid-template-columns: 1fr;
  }
}
</style>
