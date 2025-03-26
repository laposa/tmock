<script setup lang="ts">
const name = ref('');

const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const uiStore = useUiStore();

const isLoading = ref(false);

async function addNewClient() {
  isLoading.value = true;
  await clientsApi.create(name.value);
  await clientsStore.load();
  uiStore.closeDialog('client-add');
  name.value = '';
  isLoading.value = false;
}
</script>

<template>
  <div>
    <v-btn @click="uiStore.openDialog('client-add')" color="indigo">Add Client</v-btn>

    <ModalWindow id="client-add" title="Add New Client">
      <v-text-field label="Name" v-model="name" required></v-text-field>

      <template v-slot:actions>
        <v-btn 
          color="indigo"
          :disabled="isLoading" 
          :loading="isLoading"
          @click="addNewClient()">
            Create</v-btn>
        <v-btn @click="uiStore.closeDialog('client-add')">Close</v-btn>
      </template>
    </ModalWindow>
  </div>
</template>
