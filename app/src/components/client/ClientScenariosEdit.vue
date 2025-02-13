<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const scenariosStore = useScenariosStore();
const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

const props = defineProps<{
  client: Client;
}>();

scenariosStore.load();

const scenarios = computed(() => scenariosStore.list ?? []);
const isSaving = ref(false);

const clientScenariosIds = ref(props.client.scenarios?.map((s) => s.id) ?? []);

async function saveClientScenarios() {
  isSaving.value = true;

  await snackbarWrapper(
    {
      errorTitle: `Failed to update scenarios for client ${props.client.name}`,
      successMessage: `Scenarios for client <strong>${props.client.name}</strong> updated`,
    },
    async () => {
      await clientsApi.updateScenarios(props.client.id, clientScenariosIds.value);
      await clientsStore.load();
    },
  );

  isSaving.value = false;
}
</script>

<template>
  <ModalWindow id="client-scenarios" title="Edit Client Scenarios">
    <v-expansion-panels>
      <v-expansion-panel v-for="(service, index) in scenarios" :key="index">
        <v-expansion-panel-title>{{ service.path }}</v-expansion-panel-title>
        <v-expansion-panel-text>
          <v-table>
            <tbody>
              <tr v-for="scenario in service.scenarios" :key="scenario.id">
                <td width="50">
                  <v-switch
                    :value="scenario.id"
                    v-model="clientScenariosIds"
                    @update:model-value="saveClientScenarios()"
                    color="indigo"
                    hide-details
                  />
                </td>
                <td>
                  <span>{{ scenario.name }}</span>
                </td>
              </tr>
            </tbody>
          </v-table>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </ModalWindow>
</template>
