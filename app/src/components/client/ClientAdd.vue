<script setup lang="ts">
import { closeModalWindow, openModalWindow } from '@/helpers';
import { useClientsStore } from '@/stores/clients.store';
import { ref } from 'vue';

const name = ref('');

const clientsStore = useClientsStore();

async function addNewClient() {
  await clientsStore.addNewClient(name.value);
  closeModalWindow('clientAdd');
  clientsStore.load();
  name.value = '';
}
</script>

<template>
  <div>
    <v-btn @click="openModalWindow('clientAdd')" color="indigo">Add client</v-btn>
  
    <ModalWindow :id="'clientAdd'">
  
      <h2>Add new client</h2>
      <br>
      <v-text-field
        label="Name"
        v-model="name"
      ></v-text-field>

      <v-btn color="indigo" @click="addNewClient()">Create</v-btn>
  
    </ModalWindow>
  </div>
</template>
