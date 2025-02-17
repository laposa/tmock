<script setup lang="ts">

const emit = defineEmits(['confirm', 'discard'])

const uiStore = useUiStore();

function confirmChanges() {
  uiStore.closeDialog("confirmation-dialog");
  emit('confirm');
}

function discardChanges() {
  uiStore.closeDialog("confirmation-dialog");
  emit('discard');
}

</script>

<template>
  <v-dialog
    persistent
    id="confirmation-dialog"
    class="dialog"
    max-width="600"
    :model-value="uiStore.isOpen('confirmation-dialog')"
    @update:model-value="uiStore.closeDialog('confirmation-dialog')"
  >
    <v-card>
      <v-card-text>
        <slot></slot>
      </v-card-text>

      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn @click="confirmChanges()" color="red">Confirm</v-btn>
        <v-btn @click="discardChanges()">Cancel</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog> 
</template>
