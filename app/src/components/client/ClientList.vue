<script setup lang="ts">
const uiStore = useUiStore();
const clientsStore = useClientsStore();
const scenariosStore = useScenariosStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

clientsStore.load();
scenariosStore.load();

const clients = computed(() => clientsStore.list ?? []);

function openClientEdit(type: DialogType, client: Client) {
  clientsStore.setDetail(client);
  uiStore.openDialog(type);
}

async function toggleEnabled(client: Client, enabled: boolean) {
  const msg = enabled ? 'enabled' : 'disabled';

  await snackbarWrapper(
    {
      errorTitle: `Failed to toggle client ${client.name}`,
      successMessage: `Client <strong>${client.name}</strong> ${msg}`,
    },
    async () => {
      await clientsApi.updateEnabled(client.id, enabled);
    },
  );

  clientsStore.load();
  scenariosStore.load();
}

async function disableScenario(client: Client, scenarioId: number) {
  const scenarios = client.scenarios?.flatMap((s) => s.id) ?? [];

  scenarios.splice(scenarios.indexOf(scenarioId), 1);

  await snackbarWrapper(
    {
      errorTitle: `Failed to remove scenario from client`,
      successMessage: `Scenario removed from client`,
    },
    async () => {
      await clientsApi.updateScenarios(client.id, scenarios);
      await clientsStore.load();
    },
  );
}
</script>

<template>
  <main>
    <v-table theme="dark">
      <thead>
        <tr>
          <th></th>
          <th>Name</th>
          <th>Conditions</th>
          <th>Scenarios</th>
        </tr>
      </thead>

      <tbody>
        <tr v-for="client in clients" :key="client.id">
          <td>
            <v-switch
              v-model="client.enabled"
              @update:model-value="(val) => toggleEnabled(client, val || false)"
              color="indigo"
              hide-details
            />
          </td>
          <td>
            <span class="edit" @click="openClientEdit('client-edit', client)">{{ client.name }}</span>
          </td>
          <td>
            <span class="edit" @click="openClientEdit('client-conditions', client)">Conditions</span>
          </td>
          <td>
            <div class="chips">
              <!-- TODO: add some sort of default colors and then assign different colors to different services?  -->
              <v-chip
                v-for="scenario in client.scenarios"
                :key="scenario.id"
                :closable="true"
                @click:close="disableScenario(client, scenario.id)"
                ><p>
                  <b>{{ scenario.service }}</b> | {{ scenario.name }}
                </p>
              </v-chip>
              <button @click="openClientEdit('client-scenarios', client)">
                <v-icon class="client-scenarios-add" color="white" icon="mdi-plus-circle"> </v-icon>
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </v-table>
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

.chips {
  display: flex;
  flex-wrap: wrap;
  padding: 5px 0;
  gap: 5px;
  align-items: center;
}

.client-scenarios-add {
  &:before {
    transition: 0.3s ease;
  }

  &:hover {
    &:before {
      color: var(--primary-light);
    }
  }
}
</style>
