<template>
  <a-row align="stretch" :gutter="rowGutter" class="gf-page-layout" :class="getClass">
    <a-col v-if="slots.left" v-bind="props.leftColProps" :xs="0" :sm="8" :md="7" :lg="6" :xl="5" :xxl="4" flex="260px">
      <div class="gf-page-layout__left" :style="props.leftStyle">
        <slot name="left"></slot>
      </div>
    </a-col>
    <a-col :xs="24" :sm="16" :md="17" :lg="18" :xl="19" :xxl="20" flex="1" v-bind="props.rightColProps">
      <div v-if="slots.header" class="gf-page-layout__header" :style="props.headerStyle">
        <slot name="header"></slot>
      </div>
      <div class="gf-page-layout__body" :style="props.bodyStyle">
        <slot></slot>
      </div>
    </a-col>
  </a-row>
</template>

<script setup lang='ts'>
import {computed,useSlots} from 'vue';
import type { ColProps } from '@arco-design/web-vue'
import type { CSSProperties } from 'vue'

defineOptions({ name: 'PageLayout' })

const props = withDefaults(defineProps<Props>(), {
  margin: true,
  padding: true,
  gutter: false,
  leftColProps: () => ({}),
  rightColProps: () => ({}),
  leftStyle: () => ({}),
  headerStyle: () => ({}),
  bodyStyle: () => ({})
})

/** 组件插槽定义 */
defineSlots<{
  header: () => void
  left: () => void
  default: () => void
}>()

const getClass = computed(() => {
  return {
    'gf-page-layout--margin': props.margin,
    'gf-page-layout--padding': props.padding,
    'gf-page-layout--gutter': !!props.gutter
  }
})

const rowGutter = computed(() => {
  if (typeof props.gutter === 'boolean') {
    return props.gutter ? 14 : undefined
  }
  return props.gutter
})

/** 组件属性定义 */
interface Props {
  margin?: boolean
  padding?: boolean
  gutter?: boolean | number
  leftColProps?: ColProps
  rightColProps?: ColProps
  leftStyle?: CSSProperties
  headerStyle?: CSSProperties
  bodyStyle?: CSSProperties
}

const slots = useSlots()
</script>

<style lang='less' scoped>
.gf-page-layout {
  flex: 1;
  height: 100%;
  display: flex;
  overflow: hidden;
  box-sizing: border-box;
  background-color: var(--color-bg-1);

  &--margin {
    margin: @margin;
  }

  &--padding {

    .gf-page-layout__left,
    .gf-page-layout__header,
    .gf-page-layout__body {
      padding: @padding;
    }

    .gf-page-layout__header {
      padding-bottom: 0;
    }
  }

  &--gutter {
    .gf-page-layout__body-left {
      border-right: none;
    }
  }

  :deep(.arco-col) {
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
}

.gf-page-layout__left {
  height: 100%;
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
}

.gf-page-layout__header {
  border-bottom: 1px solid var(--color-border);
  box-sizing: border-box;
}

.gf-page-layout__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
}
</style>
