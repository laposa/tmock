export type DialogType = 'client-add' | 'client-edit' | 'client-conditions' | 'client-scenarios';

export const useUiStore = defineStore('ui', () => {
  const dialogs = ref<DialogType[]>([]);
  const clientsStore = useClientsStore();

  function openDialog(type: DialogType) {
    dialogs.value.push(type);
  }

  function closeDialog(type: DialogType) {
    dialogs.value = dialogs.value.filter((t) => t !== type);
    clientsStore.setDetail(null);
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
