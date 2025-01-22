export function openModalWindow(id: string) {
  const modal = document.getElementById(id) as HTMLDialogElement;
  if (modal) {
    modal.showModal();
  }
}

export function closeModalWindow(id: string) {
  const modal = document.getElementById(id) as HTMLDialogElement;
  if (modal) {
    modal.close();
  }
}
