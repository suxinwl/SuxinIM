<template>
  <VCharts
    v-if="renderChart"
    :option="option ?? options"
    theme="v5"
    :autoresize="autoResize"
    :style="{ width, height }"
  />
</template>

<script lang="ts" setup>
  import { ref, nextTick, type PropType } from 'vue';
  import type { EChartsOption } from 'echarts';
  import VCharts from 'vue-echarts';
  import { use } from 'echarts/core'
  import { PieChart, LineChart, BarChart } from 'echarts/charts'
  import { GraphicComponent, LegendComponent, TitleComponent, TooltipComponent ,GridComponent } from 'echarts/components'
  import { CanvasRenderer } from 'echarts/renderers'
  import 'echarts/theme/v5.js';

  use([GraphicComponent, TitleComponent, TooltipComponent, LegendComponent, PieChart, LineChart, BarChart, CanvasRenderer,GridComponent ])
  defineProps({
    option: {
      type: Object as PropType<EChartsOption>,
      default: undefined,
    },
    options: {
      type: Object as PropType<EChartsOption>,
      default() {
        return {};
      },
    },
    autoResize: {
      type: Boolean,
      default: true,
    },
    width: {
      type: String,
      default: '100%',
    },
    height: {
      type: String,
      default: '100%',
    },
  });
  // const appStore = useAppStore();
  // const theme = computed(() => {
  //   if (appStore.theme === 'dark') return 'dark';
  //   return '';
  // });
  const renderChart = ref(false);
  // wait container expand
  nextTick(() => {
    renderChart.value = true;
  });
</script>

<style scoped lang="less"></style>
