export type DialogType = 
  'client-add' | 
  'client-edit' | 
  'client-conditions' | 
  'client-scenarios' | 
  'scenario-edit' |
  'confirmation-dialog';

export const useUiStore = defineStore('ui', () => {
  const dialogs = ref<DialogType[]>([]);
  const clientsStore = useClientsStore();
  const scenariosStore = useScenariosStore();
  const confirmValue = ref('');

  function openDialog(type: DialogType) {
    dialogs.value.push(type);
  }

  function closeDialog(type: DialogType) {
    dialogs.value = dialogs.value.filter((t) => t !== type);
    if(type !== 'confirmation-dialog') {
      clientsStore.setDetail(null);
      scenariosStore.setDetail(null);
    }
  }

  function isOpen(type: DialogType) {
    return dialogs.value.includes(type);
  }

  function setConfirmValue(value: string) {
    confirmValue.value = value; 
  }

  return {
    dialogs,
    confirmValue,
    openDialog,
    closeDialog,
    isOpen,
    setConfirmValue,
  };
});
