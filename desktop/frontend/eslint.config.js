/**
 * ESLint flat config（eslint 10 / typescript-eslint 8）。
 *
 * 目标（F-I17 / M-DSK-TST-006）：
 *  - rules-of-hooks 拦截条件 hooks（B1 类回归）
 *  - TS 推荐规则 + 宽松的 any/unused 策略（本项目类型门禁以 tsc strict 为准，
 *    eslint 侧重结构与 hooks 纪律，不重复惩罚）
 *  - wailsjs/ 是生成物，dist/ 是构建产物，均跳过
 */
import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import reactHooks from 'eslint-plugin-react-hooks'
import globals from 'globals'

export default tseslint.config(
  {
    ignores: ['dist/**', 'wailsjs/**', 'node_modules/**', 'playwright-report/**', 'test-results/**'],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    plugins: {
      'react-hooks': reactHooks,
    },
    rules: {
      // rules-of-hooks 是核心纪律（条件 hooks 会崩溃），保持 error。
      // v7 新规则 set-state-in-effect / static-components / incompatible-library
      // 偏 React 19 迁移建议，对现有 React 18 代码是性能建议而非正确性，
      // 降为 off，避免大规模无关重构；exhaustive-deps / refs 降为 warn。
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/refs': 'warn',
      'react-hooks/static-components': 'off',
      'react-hooks/incompatible-library': 'off',
      // 项目策略：any 由 tsc strict 把关；unused 变量 warn（_ 前缀豁免）
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      // 浏览器脚本常用但 eslint 严格模式报错，交给 tsc/运行时
      'no-undef': 'off',
    },
  },
)
