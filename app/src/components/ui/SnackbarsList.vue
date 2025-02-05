<script setup lang="ts">
const snackbarsStore = useSnackbarsStore();

const handleClick = (item: SnackbarItem) => {
  if (item.closeOnClick) {
    snackbarsStore.hideSnackbar(item);
  }
  if (item.action) {
    item.action();
  }
};
</script>

<template>
  <div id="snackbar-list">
    <v-alert
      v-for="(item, key) in snackbarsStore.snackbars"
      :key="key"
      :type="item.type"
      :icon="'$' + item.type"
      :title="item.title"
      @click="handleClick(item)"
      :class="{ pointer: !!item.action, 'pre-wrap': item.preWrap }"
      location="top right"
    >
      <div class="snackbar-message" v-html="item.message"></div>

      <v-btn
        v-if="item.permanent"
        variant="outlined"
        size="small"
        @click="() => snackbarsStore.hideSnackbar(item)"
        >Dismiss</v-btn
      >
    </v-alert>
  </div>
</template>

<style scoped>
#snackbar-list {
  position: fixed;
  top: 10px;
  right: 10px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
  max-height: 100%;
  max-width: 40vw;
}

#snackbar-list:deep(.v-btn) {
  margin-top: 10px;
}

#snackbar-list:deep(.v-alert.pointer) {
  cursor: pointer;
}

#snackbar-list:deep(.v-alert.pre-wrap .snackbar-message) {
  white-space: pre-wrap;
}
</style>
