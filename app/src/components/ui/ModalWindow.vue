<script setup lang="ts">
import type { DialogType } from '@/stores/ui.store';

const props = defineProps<{ id: DialogType; title?: string }>();

const uiStore = useUiStore();
</script>

<template>
  <v-dialog
    max-width="800"
    :model-value="uiStore.isOpen(props.id)"
    @update:model-value="uiStore.closeDialog(props.id)"
  >
    <v-card :title="title">
      <v-card-text>
        <slot></slot>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style scoped>
dialog {
  background-color: #333333;
  margin: 10% auto 0;
  width: 80%;
  max-height: 80%;
  border: none;
  outline: none;
  border-radius: 10px;
  box-shadow:
    0 0 #0000,
    0 0 #0000,
    0 25px 50px -12px rgba(0, 0, 0, 0.25);
  padding: 1.6rem;
  color: white;
}

.close {
  position: absolute;
  line-height: 1;
  top: 12px;
  right: 20px;
  font-size: 1.6rem;
  opacity: 0.5;
  outline: none;
  border: none;
}
</style>
