<script setup lang="ts">
const clientsStore = useClientsStore();
</script>

<template>
  <main>
    <div class="heading">
      <h1>Clients</h1>
      <ClientAdd></ClientAdd>
    </div>
    <ClientList></ClientList>
    <ClientEdit v-if="clientsStore.detail" :client="clientsStore.detail"> </ClientEdit>
    <ClientConditionsEdit
      v-if="clientsStore.detail"
      :client="clientsStore.detail"
    ></ClientConditionsEdit>
    <ClientConditionsEdit
      v-if="clientsStore.detail"
      :client="clientsStore.detail"
    ></ClientConditionsEdit>
    <ClientScenariosEdit
      v-if="clientsStore.detail"
      :client="clientsStore.detail"
    ></ClientScenariosEdit>
  </main>
</template>
