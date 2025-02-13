<script setup lang="ts">
import type { DialogType } from '@/stores/ui.store';

const props = defineProps<{ id: DialogType; title?: string }>();

const uiStore = useUiStore();
</script>

<template>
  <v-dialog
    class="dialog"
    max-width="800"
    :model-value="uiStore.isOpen(props.id)"
    @update:model-value="uiStore.closeDialog(props.id)"
  >
    <v-card :title="title">
      <v-card-text>
        <slot></slot>
      </v-card-text>

      <v-card-actions>
        <v-spacer></v-spacer>
        <slot name="actions"></slot>
        <v-btn @click="uiStore.closeDialog(props.id)">Close</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
