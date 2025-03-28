<script setup lang="ts">
const uiStore = useUiStore();
const scenariosStore = useScenariosStore();
const scenariosApi = useScenariosApi();

scenariosStore.load();

const isDetailLoaded = ref(false);

const scenarios = computed(() => scenariosStore.list.flatMap((s) => s.scenarios) ?? []);

const groupBy = [
  {
    key: 'service'
  }
];

const headers = [
  { title: '', key: 'data-table-group', width: '50px' },
  { title: 'Name', key: 'name', width: '30%'},
  { title: 'Method', key: 'requestMethod', width: '100px' },
  { title: 'Path', key: 'requestPath'},
  { title: 'Condition', key: 'requestCOndition'},
  { title: 'Code', key: 'responseCode', width: '80px'},
  { title: 'Headers', key: 'responseHeaders', width: '130px' },
  { title: 'Body', key: 'responseBody', width: '120px' },
];

async function openScenarioEdit(type: DialogType, scenarioId: number, target?: string) {
  isDetailLoaded.value = false;
  uiStore.openDialog(type);

  await scenariosApi.getById(scenarioId.toString()).then((response) => {
    scenariosStore.setDetail(response.data.scenario);
  }); 

  isDetailLoaded.value = true;

  if (target) {
    const element = document.querySelector(target);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth' });
    }
  }
}

async function openNewScenario() {
  isDetailLoaded.value = false;
  uiStore.openDialog('scenario-modal');

  await scenariosStore.setDetail({
    id: 0,
    name: '',
    service: '',
    requestMethod: null,
    requestPath: null,
    requestCondition: null,
    responseCode: null,
    responseHeaders: null,
    responseBody: null,
    skipProxy: false,
  });

  isDetailLoaded.value = true;
}

</script>

<template>
  <main>
    <div class="heading">
      <h1>Scenarios</h1>
      <v-btn 
        color="indigo"
        @click="openNewScenario()">
          Add Scenario
      </v-btn>
    </div>

    <v-data-table
      :group-by="groupBy"
      :headers="headers"
      :items="scenarios"
      items-per-page="0"
      theme="dark"
      hide-default-footer
    >
      <template v-slot:group-header="{ item, columns, toggleGroup, isGroupOpen }">
        <tr>
          <td :colspan="columns.length">
            <v-btn
              :icon="isGroupOpen(item) ? '$expand' : '$next'"
              size="small"
              variant="text"
              @click="toggleGroup(item)"
            ></v-btn>
            <span 
              class="service" 
              :class="{ open: isGroupOpen(item) }"
              @click="toggleGroup(item)">
              {{ item.value ?? 'Unspecified service' }}
            </span>
          </td>
        </tr>
      </template>

      <template v-slot:item="{ item }">
        <tr>
          <td></td>
          <td 
            class="scenario-name"
            @click="openScenarioEdit('scenario-modal', item.id)">
            {{ item.name }}
          </td>
          <td>{{ item.requestMethod }}</td>
          <td>{{ item.requestPath }}</td>
          <td>{{ item.requestCondition }}</td>
          <td>{{ item.responseCode }}</td>
          <td 
            class="scenario-name"
            @click="openScenarioEdit('scenario-modal', item.id, '#responseHeaders')">
              {{ item.responseHeaders ? 'Show Headers' : '' }}
              <v-tooltip
                v-if="item.responseHeaders"
                activator="parent"
                location="end">
                  <pre>{{ item.responseHeaders }}</pre>
              </v-tooltip>
          </td>
          <td 
            class="scenario-name"
            @click="openScenarioEdit('scenario-modal', item.id, '#responseBody')">
              {{ item.responseBody ? 'Show Body' : '' }}
              <v-tooltip
                v-if="item.responseBody"
                activator="parent"
                location="end">
                  <pre>{{ item.responseBody }}</pre>
              </v-tooltip>
          </td>
        </tr>
      </template>
    </v-data-table>
    <ScenarioModal 
      v-if="uiStore.dialogs.includes('scenario-modal')"
      :detail-loading="!isDetailLoaded">
    </ScenarioModal>
  </main>
</template>

<style scoped>
.v-table {
  border-radius: 10px;
}

.edit {
  color: var(--primary);
  cursor: pointer;
  transition: 0.3s;
}

.edit:hover {
  color: white;
}

.service.open {
  color: var(--primary);
}

.service {
  top: 3px;
  left: 10px;
  position:relative;
  font-size: 18px;
  font-weight: 500;
  transition: 0.3s ease;
  cursor: pointer;

  &:hover {
    color: var(--primary-light);
  }
}

.scenario-name {
  font-weight: 500;
  cursor: pointer;
  transition: 0.3s ease;

  &:hover {
    color: var(--primary-light);
  }
}
</style>
