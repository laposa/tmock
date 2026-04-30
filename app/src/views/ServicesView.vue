<script setup lang="ts">
const servicesStore = useServicesStore();
</script>

<template>
  <main>
    <div class="heading">
      <h1>Services</h1>
      <ServiceAdd></ServiceAdd>
    </div>
    <ServiceList></ServiceList>
    <ServiceEdit v-if="servicesStore.detail" :service="servicesStore.detail"></ServiceEdit>
  </main>
</template>
