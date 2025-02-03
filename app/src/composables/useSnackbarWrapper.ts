export type SnackbarCatchErrorOpts = {
  errorTitle?: string;
  successTitle?: string;
  successMessage?: string;
};

export function useSnackbarWrapper() {
  const snackbarsStore = useSnackbarsStore();

  async function snackbarWrapper<T>(
    { errorTitle, successTitle, successMessage }: SnackbarCatchErrorOpts,
    fn: () => Promise<T>,
    catchFn?: (error: unknown) => void,
  ) {
    try {
      const result = await fn();
      if (successMessage) {
        snackbarsStore.displaySnackbar({
          title: successTitle,
          message: successMessage,
          type: 'success',
        });
      }

      return result;
    } catch (error) {
      // do not display canceled errors
      if (error?.code !== 'ERR_CANCELED') {
        let message = error.message;
        if (error.response && error.response.data) {
          if (error.response.data.details) {
            message = error.response.data.details.join('<br>');
          } else if (error.response.data.message) {
            message = error.response.data.message;
          }
        }

        snackbarsStore.displaySnackbar({
          title: errorTitle,
          message,
          type: 'error',
        });
      }

      catchFn?.(error);
    }
  }

  return { snackbarWrapper };
}
