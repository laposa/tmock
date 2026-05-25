<script setup lang="ts">
const name = ref('');
const upstreamUrl = ref('');
const path = ref('');

const servicesStore = useServicesStore();
const servicesApi = useServicesApi();
const uiStore = useUiStore();

const isLoading = ref(false);

async function addNewService() {
  isLoading.value = true;
  await servicesApi.create({
    name: name.value,
    upstreamUrl: upstreamUrl.value,
    path: path.value,
  });
  await servicesStore.load();
  uiStore.closeDialog('service-add');
  name.value = '';
  upstreamUrl.value = '';
  path.value = '';
  isLoading.value = false;
}
</script>

<template>
  <div>
    <v-btn @click="uiStore.openDialog('service-add')" color="indigo">Add Service</v-btn>

    <ModalWindow id="service-add" title="Add New Service">
      <v-text-field label="Name" v-model="name" required></v-text-field>
      <v-text-field label="Upstream URL" v-model="upstreamUrl" required></v-text-field>
      <v-text-field label="Path" v-model="path" required></v-text-field>

      <template v-slot:actions>
        <v-btn @click="uiStore.closeDialog('service-add')">Close</v-btn>
        <v-btn
          color="indigo"
          :disabled="isLoading"
          :loading="isLoading"
          @click="addNewService()">
            Create</v-btn>
      </template>
    </ModalWindow>
  </div>
</template>
