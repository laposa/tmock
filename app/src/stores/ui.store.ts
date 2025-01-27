export type DialogType = 'client-add' | 'client-edit' | 'client-conditions';

export const useUiStore = defineStore('ui', () => {
  const dialogs = ref<DialogType[]>([]);

  function openDialog(type: DialogType) {
    dialogs.value.push(type);
  }

  function closeDialog(type: DialogType) {
    dialogs.value = dialogs.value.filter((t) => t !== type);
  }

  function isOpen(type: DialogType) {
    return dialogs.value.includes(type);
  }

  return {
    dialogs,
    openDialog,
    closeDialog,
    isOpen,
  };
});
