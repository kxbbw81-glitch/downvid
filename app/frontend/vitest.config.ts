import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// 与仓库根同一个理由：**不引 jsdom**。
// 只测纯逻辑（词表、占位符替换、语言探测），DOM 相关的靠真机验证。
//
// backend 的适配测试会加载响应式 i18n，需编译 .svelte.ts 的 runes。
// 仍使用 Node 环境，不模拟 DOM。
export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
