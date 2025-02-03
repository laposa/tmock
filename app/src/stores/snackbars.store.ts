import { reactive, ref } from 'vue';
import { defineStore } from 'pinia';

export type SnackbarType = 'success' | 'error' | 'info' | 'warning';

export interface SnackbarItem {
  message: string;
  type: SnackbarType;
  fullMessage?: string;
  title?: string;
  uid?: string;
  permanent?: boolean;
  closeOnClick?: boolean;
  timeout?: number;
  preWrap?: boolean;
  action?: () => void;
}

export const useSnackbarsStore = defineStore('snackbars', () => {
  const snackbars = reactive<SnackbarItem[]>([]);
  const snackbarDetail = ref<SnackbarItem | null>(null);
  const defaultTimeout = 5000;

  function displaySnackbar(snackbar: SnackbarItem) {
    if (snackbar.uid && snackbars.find((s) => s.uid === snackbar.uid)) {
      return;
    }

    snackbars.push(snackbar);

    // do not check length for permanent snackbars
    if (snackbar.permanent) {
      return;
    }

    setTimeout(() => {
      hideSnackbar(snackbar);
    }, snackbar.timeout || defaultTimeout);
  }

  function reset() {
    snackbars.splice(0, snackbars.length);
  }

  function hideSnackbar(snackbar: SnackbarItem) {
    const index = snackbars.indexOf(snackbar);
    if (index === -1) {
      return;
    }

    snackbars.splice(index, 1);
  }

  return {
    snackbars,
    snackbarDetail,
    defaultTimeout,
    displaySnackbar,
    hideSnackbar,
    reset,
  };
});
