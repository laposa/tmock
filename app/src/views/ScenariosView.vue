<script setup lang="ts">
const scenariosStore = useScenariosStore();
</script>

<template>
  <main>
    <div class="heading">
      <h1>Scenarios</h1>
      <ScenarioAdd></ScenarioAdd>
    </div>
    <ScenarioList></ScenarioList>
    <ScenarioEdit 
      v-if="scenariosStore.detail" 
      :scenario="scenariosStore.detail">
    </ScenarioEdit>
  </main>
</template>
