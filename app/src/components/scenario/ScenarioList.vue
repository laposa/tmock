<script setup lang="ts">
const uiStore = useUiStore();
const scenariosStore = useScenariosStore();

scenariosStore.load();

const scenarios = computed(() => scenariosStore.list.flatMap((s) => s.scenarios) ?? []);

const groupBy = [
  {
    key: 'service'
  }
];

const headers = [
  { title: 'Name', key: 'name', class: 'ahoj' },
  { title: 'Method', key: 'requestMethod' },
  { title: 'Path', key: 'requestPath' },
  { title: 'Condition', key: 'requestCOndition' },
  { title: 'Code', key: 'responseCode' },
  { title: 'Headers', key: 'responseHeaders' },
  { title: 'Body', key: 'responseBody' },
];

async function openScenarioEdit(type: DialogType, scenario: Scenario, target?: string) {
  scenariosStore.setDetail(scenario);
  await uiStore.openDialog(type);

  if (target) {
    const element = document.querySelector(target);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth' });
    }
  }
}

</script>

<template>
  <main>
    <v-data-table
      :group-by="groupBy"
      :headers="headers"
      :items="scenarios"
      items-per-page="50"
      theme="dark"
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
              :class="{ open: isGroupOpen(item) }">
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
            @click="openScenarioEdit('scenario-modal', item)">
            {{ item.name }}
          </td>
          <td>{{ item.requestMethod }}</td>
          <td>{{ item.requestPath }}</td>
          <td>{{ item.requestCondition }}</td>
          <td>{{ item.responseCode }}</td>
          <td 
            class="scenario-name"
            @click="openScenarioEdit('scenario-modal', item, '#responseHeaders')">
              {{ item.responseHeaders ? 'Show headers' : '' }}
              <v-tooltip
                activator="parent"
                location="end">
                  <pre>{{ item.responseHeaders }}</pre>
              </v-tooltip>
          </td>
          <td 
            class="scenario-name"
            @click="openScenarioEdit('scenario-modal', item, '#responseBody')">
              {{ item.responseBody ? 'Show body' : '' }}
              <v-tooltip
                activator="parent"
                location="end">
                  <pre>{{ item.responseBody }}</pre>
              </v-tooltip>
          </td>
        </tr>
      </template>
    </v-data-table>
  </main>
</template>

<style scoped>
.v-table {
  border-radius: 10px;
}

td:first-of-type,
th:first-of-type {
  width: 60px;
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
