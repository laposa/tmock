<script setup lang="ts">
const uiStore = useUiStore();
const clientsStore = useClientsStore();
const scenariosStore = useScenariosStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

clientsStore.load();
scenariosStore.load();

const clients = computed(() => clientsStore.list ?? []);

function openClientEdit(client: Client) {
  clientsStore.setDetail(client);
  uiStore.openDialog('client-edit');
}

function openClientConditionsEdit(client: Client) {
  clientsStore.setDetail(client);
  uiStore.openDialog('client-conditions');
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
            <span class="edit" @click="openClientEdit(client)">{{ client.name }}</span>
          </td>
          <td><span class="edit" @click="openClientConditionsEdit(client)">Conditions</span></td>
          <!-- TODO scenario chips when scenarios are implemented -->
          <td>{{ client.scenarios }}</td>
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
</style>
