<template>
  <div class="app-header-wrapper">
    <header class="app-header" :class="{ 'header-fixed': fixed }">
      <div class="header-content">
        <!-- Left section: Logo + Left slot -->
        <div class="header-left">
          <router-link v-if="showLogo" to="/" class="logo">
            <span class="logo-text">🎬 星月 Studio</span>
          </router-link>
          <!-- Left slot for business content | 左侧插槽用于业务内容 -->
          <slot name="left" />
        </div>

        <!-- Center section: Center slot -->
        <div class="header-center">
          <slot name="center" />
        </div>

        <!-- Right section: Actions + Right slot -->
        <div class="header-right">
          
          <!-- Language Switcher | 语言切换 -->
          <LanguageSwitcher v-if="showLanguage" />
          
          <!-- Theme Toggle | 主题切换 -->
          <ThemeToggle v-if="showTheme" />
          
          <!-- AI Config (Model Switch) | AI 配置（模型切换） -->
          <el-button v-if="showAIConfig" class="header-btn" @click="handleOpenAIConfig">
            <el-icon><Setting /></el-icon>
            <span class="btn-text">{{ $t('drama.aiConfig') }}</span>
          </el-button>
          <!-- Right slot for business content (before actions) | 右侧插槽（在操作按钮前） -->
          <slot name="right" />
        </div>
      </div>
    </header>
    
    <!-- AI Config Dialog | AI 配置对话框 -->
    <AIConfigDialog v-model="showConfigDialog" @config-updated="emit('config-updated')" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Setting } from '@element-plus/icons-vue'
import ThemeToggle from './ThemeToggle.vue'
import AIConfigDialog from './AIConfigDialog.vue'
import LanguageSwitcher from '@/components/LanguageSwitcher.vue'

/**
 * AppHeader - Global application header component
 * 应用顶部头组件
 * 
 * Features | 功能:
 * - Fixed position at top | 固定在顶部
 * - Model/Theme/Language switch | 模型/主题/语言切换
 * - Slots support for business content | 支持插槽放置业务内容
 * 
 * Slots | 插槽:
 * - left: Content after logo | logo 右侧内容
 * - center: Center content | 中间内容
 * - right: Content before actions | 操作按钮左侧内容
 */

interface Props {
  /** Fixed position at top | 是否固定在顶部 */
  fixed?: boolean
  /** Show logo | 是否显示 logo */
  showLogo?: boolean
  /** Show language switcher | 是否显示语言切换 */
  showLanguage?: boolean
  /** Show theme toggle | 是否显示主题切换 */
  showTheme?: boolean
  /** Show AI config button | 是否显示 AI 配置按钮 */
  showAIConfig?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  fixed: true,
  showLogo: true,
  showLanguage: true,
  showTheme: true,
  showAIConfig: true
})

const emit = defineEmits<{
  (e: 'open-ai-config'): void
  (e: 'config-updated'): void
}>()

// AI Config dialog state | AI 配置对话框状态
const showConfigDialog = ref(false)

// Handle open AI config | 处理打开 AI 配置
const handleOpenAIConfig = () => {
  showConfigDialog.value = true
  emit('open-ai-config')
}

// Expose methods for external control | 暴露方法供外部控制
defineExpose({
  openAIConfig: () => {
    showConfigDialog.value = true
  }
})
</script>

<style scoped>
.app-header {
  background: var(--bg-card);
  border-bottom: 1px solid var(--border-primary);
  backdrop-filter: blur(8px);
  z-index: 1000;
}

.app-header.header-fixed {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
}

.header-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--space-4);
  height: 70px;
  max-width: 100%;
  margin: 0 auto;
}

.header-left {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  flex-shrink: 0;
}

.header-center {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  min-width: 0;
}

.header-right {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}

.logo {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  text-decoration: none;
  color: var(--text-primary);
  font-weight: 700;
  font-size: 1.125rem;
  transition: opacity var(--transition-fast);
}

.logo:hover {
  opacity: 0.8;
}

.logo-text {
  background: linear-gradient(135deg, var(--accent) 0%, #06b6d4 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.header-btn {
  border-radius: var(--radius-lg);
  font-weight: 500;
}

.header-btn .btn-text {
  margin-left: 4px;
}

/* Dark mode adjustments | 深色模式适配 */
.dark .app-header {
  background: rgba(26, 33, 41, 0.95);
}

/* ========================================
   Common Slot Styles / 插槽通用样式
   ======================================== */

/* Back Button | 返回按钮 */
:deep(.back-btn) {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 12px;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
  border-radius: var(--radius-md);
  transition: all var(--transition-fast);
}

:deep(.back-btn:hover) {
  color: var(--text-primary);
  background: var(--bg-hover);
}

:deep(.back-btn .el-icon) {
  font-size: 1rem;
}

/* Page Title | 页面标题 */
:deep(.page-title) {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

:deep(.page-title h1),
:deep(.header-title),
:deep(.drama-title) {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.3;
}

:deep(.page-title .subtitle) {
  font-size: 0.8125rem;
  color: var(--text-muted);
}

/* Episode Title | 章节标题 */
:deep(.episode-title) {
  font-size: 1rem;
  font-weight: 600;
  color: var(--text-primary);
}

/* Responsive | 响应式 */
@media (max-width: 768px) {
  .header-content {
    padding: 0 var(--space-3);
  }
  
  .btn-text {
    display: none;
  }
  
  .header-btn {
    padding: 8px;
  }

  :deep(.page-title h1),
  :deep(.header-title),
  :deep(.drama-title) {
    font-size: 1rem;
  }

  :deep(.back-btn span) {
    display: none;
  }
}
</style>
