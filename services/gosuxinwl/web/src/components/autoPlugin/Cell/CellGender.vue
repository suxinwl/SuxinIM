<template>
  <a-tag v-if="Number(props.gender) === 1" color="arcoblue" size="small" class="cell_round">
    <template #icon><icon-man /></template>
    <template #default>男</template>
  </a-tag>
  <a-tag v-else-if="Number(props.gender) === 2" color="magenta" size="small" class="cell_round">
    <template #icon><icon-woman /></template>
    <template #default>女</template>
  </a-tag>
  <a-tag v-else color="gray" size="small" class="cell_round">
    <template #icon><icon-lock /></template>
    <template #default>密</template>
  </a-tag>
</template>

<script lang="ts" setup>
const props = withDefaults(defineProps<Props>(), {
  gender: 1
})
interface Props {
  gender: 1 | 2 | 0
}
</script>

