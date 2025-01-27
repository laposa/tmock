<script setup lang="ts">
const name = ref('');

const clientsStore = useClientsStore();
const uiStore = useUiStore();

async function addNewClient() {
  await clientsStore.addNewClient(name.value);
  uiStore.closeDialog('client-add');
  clientsStore.load();
  name.value = '';
}
</script>

<template>
  <div>
    <v-btn @click="uiStore.openDialog('client-add')" color="indigo">Add client</v-btn>

    <ModalWindow id="client-add" title="Add new client">
      <v-text-field label="Name" v-model="name"></v-text-field>

      <v-btn color="indigo" @click="addNewClient()">Create</v-btn>
    </ModalWindow>
  </div>
</template>
