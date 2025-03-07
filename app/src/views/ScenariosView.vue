<script setup lang="ts">
const scenariosStore = useScenariosStore();
const uiStore = useUiStore();

function openNewScenario() {
  scenariosStore.setDetail({
    id: 0,
    name: '',
    service: '',
    requestMethod: null,
    requestPath: null,
    requestCondition: null,
    responseCode: null,
    responseHeaders: null,
    responseBody: null,
  });
  uiStore.openDialog('scenario-modal');
}
</script>

<template>
  <main>
    <div class="heading">
      <h1>Scenarios</h1>
      <v-btn 
        color="indigo"
        @click="openNewScenario()">
          Add scenario
      </v-btn>
    </div>
    <ScenarioList></ScenarioList>
    <ScenarioModal 
        v-if="scenariosStore.detail"
        :scenario="scenariosStore.detail"
        :allow-add="true">
      </ScenarioModal>
  </main>
</template>
