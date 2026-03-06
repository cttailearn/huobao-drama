<template>
  <div class="page-container">
    <div class="content-wrapper">
      <AppHeader :fixed="false">
        <template #left>
          <div class="page-title">
            <h1>项目工作台</h1>
            <span class="subtitle">{{ drama?.title || '加载中...' }}</span>
          </div>
        </template>
        <template #right>
          <el-button @click="router.push('/')">返回项目列表</el-button>
          <el-button type="primary" @click="router.push(`/dramas/${dramaId}/novels`)">小说工作区</el-button>
        </template>
      </AppHeader>

      <div v-loading="loading" class="workbench-body">
        <el-row :gutter="16">
          <el-col :span="8">
            <el-card>
              <template #header>
                <div class="card-title">短剧进度</div>
              </template>
              <div class="stat-row">
                <div class="stat-item">
                  <div class="label">章节脚本已就绪</div>
                  <div class="value">{{ dramaScriptReadyCount }} / {{ dramaEpisodeCount }}</div>
                </div>
                <div class="stat-item">
                  <div class="label">分镜已创建</div>
                  <div class="value">{{ dramaStoryboardReadyCount }} / {{ dramaEpisodeCount }}</div>
                </div>
              </div>
              <el-progress :percentage="dramaProgressPercent" />
            </el-card>

            <el-card class="novel-progress-card">
              <template #header>
                <div class="card-title">小说进度</div>
              </template>
              <div class="project-select">
                <el-select v-model="selectedNovelId" style="width: 100%" placeholder="请选择项目内小说">
                  <el-option v-for="item in novels" :key="item.id" :label="item.title" :value="item.id" />
                </el-select>
              </div>
              <template v-if="selectedNovel">
                <div class="stat-row">
                  <div class="stat-item">
                    <div class="label">章节内容已完成</div>
                    <div class="value">{{ novelContentReadyCount }} / {{ selectedNovel.chapter_count }}</div>
                  </div>
                  <div class="stat-item">
                    <div class="label">目录已完成</div>
                    <div class="value">{{ novelOutlineReadyCount }} / {{ selectedNovel.chapter_count }}</div>
                  </div>
                </div>
                <el-progress :percentage="novelProgressPercent" />
              </template>
              <el-empty v-else description="当前项目暂无小说" :image-size="80" />
            </el-card>
          </el-col>

          <el-col :span="16">
            <el-card>
              <template #header>
                <div class="chapter-header">
                  <span>小说章节 → 短剧任务</span>
                  <el-button text @click="loadWorkbench">刷新</el-button>
                </div>
              </template>
              <template v-if="selectedNovel && selectedNovel.chapters.length > 0">
                <el-table :data="selectedNovel.chapters" style="width: 100%">
                  <el-table-column prop="chapter_number" label="章" width="70" />
                  <el-table-column prop="title" label="章节标题" min-width="220" />
                  <el-table-column label="内容状态" width="130">
                    <template #default="{ row }">
                      <el-tag :type="hasChapterContent(row) ? 'success' : 'info'">
                        {{ hasChapterContent(row) ? '已就绪' : '待补充' }}
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="任务状态" width="150">
                    <template #default="{ row }">
                      <el-tag v-if="chapterTaskState[row.chapter_number]" :type="taskTagType(chapterTaskState[row.chapter_number].status)">
                        {{ chapterTaskState[row.chapter_number].status }}
                      </el-tag>
                      <span v-else>-</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="280">
                    <template #default="{ row }">
                      <el-button
                        type="primary"
                        size="small"
                        :disabled="!hasChapterContent(row)"
                        :loading="syncingChapterNumber === row.chapter_number"
                        @click="handleCreateDramaChapterTask(row)"
                      >
                        一键生成短剧章节任务
                      </el-button>
                      <el-button size="small" @click="goEpisode(row.chapter_number)">进入章节制作</el-button>
                    </template>
                  </el-table-column>
                </el-table>
              </template>
              <el-empty v-else description="请先创建并选择小说" :image-size="100" />
            </el-card>
          </el-col>
        </el-row>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { AppHeader } from "@/components/common";
import { dramaAPI } from "@/api/drama";
import { novelAPI } from "@/api/novel";
import { taskAPI, type AsyncTask } from "@/api/task";
import type { Drama } from "@/types/drama";
import type { Novel, NovelChapter } from "@/types/novel";

const route = useRoute();
const router = useRouter();

const dramaId = computed(() => String(route.params.id || ""));
const loading = ref(false);
const drama = ref<Drama | null>(null);
const novels = ref<Novel[]>([]);
const selectedNovelId = ref<number | null>(null);
const selectedNovel = ref<Novel | null>(null);
const syncingChapterNumber = ref<number | null>(null);
const chapterTaskState = ref<Record<number, AsyncTask>>({});

const dramaEpisodeCount = computed(() => drama.value?.episodes?.length || 0);
const dramaScriptReadyCount = computed(
  () => drama.value?.episodes?.filter((ep) => !!(ep.script_content && String(ep.script_content).trim())).length || 0,
);
const dramaStoryboardReadyCount = computed(
  () => drama.value?.episodes?.filter((ep) => (ep.storyboards?.length || 0) > 0).length || 0,
);
const dramaProgressPercent = computed(() => {
  if (!dramaEpisodeCount.value) return 0;
  return Math.min(100, Math.round((dramaStoryboardReadyCount.value / dramaEpisodeCount.value) * 100));
});

const novelContentReadyCount = computed(
  () =>
    selectedNovel.value?.chapters.filter((ch) =>
      !!((ch.final_content && ch.final_content.trim()) || (ch.draft_content && ch.draft_content.trim())),
    ).length || 0,
);
const novelOutlineReadyCount = computed(
  () => selectedNovel.value?.chapters.filter((ch) => !!(ch.outline && ch.outline.trim())).length || 0,
);
const novelProgressPercent = computed(() => {
  if (!selectedNovel.value?.chapter_count) return 0;
  return Math.min(100, Math.round((novelContentReadyCount.value / selectedNovel.value.chapter_count) * 100));
});

const hasChapterContent = (chapter: NovelChapter) =>
  !!((chapter.final_content && chapter.final_content.trim()) || (chapter.draft_content && chapter.draft_content.trim()));

const taskTagType = (status: AsyncTask["status"]) => {
  if (status === "completed") return "success";
  if (status === "failed") return "danger";
  if (status === "processing") return "warning";
  return "info";
};

const loadWorkbench = async () => {
  if (!dramaId.value) return;
  loading.value = true;
  try {
    const [dramaData, novelListRes] = await Promise.all([
      dramaAPI.get(dramaId.value),
      novelAPI.list({ page: 1, page_size: 200, drama_id: Number(dramaId.value) }),
    ]);
    drama.value = dramaData;
    novels.value = novelListRes.items || [];
    if (selectedNovelId.value == null && novels.value.length > 0) {
      selectedNovelId.value = Number(novels.value[0].id);
    }
    if (selectedNovelId.value != null) {
      selectedNovel.value = await novelAPI.get(selectedNovelId.value);
    } else {
      selectedNovel.value = null;
    }
  } catch (error: any) {
    ElMessage.error(error.message || "加载工作台失败");
  } finally {
    loading.value = false;
  }
};

const handleCreateDramaChapterTask = async (chapter: NovelChapter) => {
  if (!selectedNovelId.value || !drama.value) return;
  if (!hasChapterContent(chapter)) {
    ElMessage.warning(`第${chapter.chapter_number}章内容为空，请先完善章节内容`);
    return;
  }
  syncingChapterNumber.value = chapter.chapter_number;
  try {
    await novelAPI.applyToDrama(selectedNovelId.value, drama.value.id);
    const latestDrama = await dramaAPI.get(drama.value.id);
    drama.value = latestDrama;
    const episode = latestDrama.episodes?.find((ep) => ep.episode_number === chapter.chapter_number);
    if (!episode) {
      throw new Error("未找到对应短剧章节，请先检查章节同步");
    }
    const taskRes = await dramaAPI.generateStoryboard(String(episode.id));
    if (taskRes && taskRes.task_id) {
      const status = await taskAPI.getStatus(taskRes.task_id);
      chapterTaskState.value[chapter.chapter_number] = status;
    }
    ElMessage.success(`第${chapter.chapter_number}章短剧任务已创建`);
  } catch (error: any) {
    ElMessage.error(error.message || "创建章节任务失败");
  } finally {
    syncingChapterNumber.value = null;
  }
};

const goEpisode = (chapterNumber: number) => {
  if (!drama.value) return;
  router.push(`/dramas/${String(drama.value.id)}/episode/${chapterNumber}`);
};

watch(selectedNovelId, async (id) => {
  if (id == null) {
    selectedNovel.value = null;
    return;
  }
  try {
    selectedNovel.value = await novelAPI.get(id);
  } catch (error: any) {
    ElMessage.error(error.message || "加载小说详情失败");
  }
});

onMounted(() => {
  loadWorkbench();
});
</script>

<style scoped>
.page-container {
  min-height: 100vh;
  background: var(--bg-primary);
}

.content-wrapper {
  width: 100%;
}

.page-title {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.page-title h1 {
  margin: 0;
  font-size: 1.25rem;
}

.subtitle {
  font-size: 0.85rem;
  color: var(--el-text-color-secondary);
}

.workbench-body {
  padding: 12px;
}

.card-title {
  font-weight: 600;
}

.novel-progress-card {
  margin-top: 16px;
}

.stat-row {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 12px;
}

.stat-item .label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.stat-item .value {
  font-size: 18px;
  font-weight: 600;
}

.project-select {
  margin-bottom: 12px;
}

.chapter-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

@media (max-width: 1200px) {
  .el-col {
    width: 100% !important;
    max-width: 100%;
    flex: 0 0 100%;
  }
}
</style>
