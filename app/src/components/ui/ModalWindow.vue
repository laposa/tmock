<script setup lang="ts">
const props = defineProps({
  id: String,
});

onMounted(() => {
  const dialog = document.getElementById(props.id ?? '') as HTMLDialogElement; 

  dialog.addEventListener('click', function(event) {
    const rect = dialog.getBoundingClientRect();
    const isInDialog = (rect.top <= event.clientY && event.clientY <= rect.top + rect.height &&
      rect.left <= event.clientX && event.clientX <= rect.left + rect.width);
    if (!isInDialog) {
      dialog.close();
    }
  });
})

function closeDialog() {
  const dialog = document.getElementById(props.id ?? '') as HTMLDialogElement;
  dialog.close();
}
</script>

<template>
  <dialog :id="props.id">
    <button class="close" @click="closeDialog()">×</button>
    <slot></slot>
  </dialog>
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
  box-shadow: 0 0 #0000, 0 0 #0000, 0 25px 50px -12px rgba(0, 0, 0, 0.25);
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
