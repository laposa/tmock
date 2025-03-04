<script setup lang="ts">
import type { ResponseHeaderItem } from './headers/ResponseHeaderItem.vue';

const uiStore = useUiStore();
const scenariosStore = useScenariosStore();
const scenariosApi = useScenariosApi();
const { snackbarWrapper } = useSnackbarWrapper();

const name = ref('');
const service = ref('');
const requestMethod: Ref<string | null> = ref(null);
const requestPath: Ref<string | null> = ref(null);
const requestCondition: Ref<string | null> = ref(null);
const responseCode: Ref<number | null> = ref(null);
const responseHeaders: Ref<ResponseHeaderItem[] | null> = ref(null);

function arrayToRecord(array: ResponseHeaderItem[]): Record<string, string> {
  return array.reduce((acc, { header, value }) => {
    acc[header] = value;
    return acc;
  }, {} as Record<string, string>);
}

async function createScenario() {
  snackbarWrapper(
    {
      errorTitle: `Failed to save changes to add new scenario`,
      successMessage: `Scenario <strong>${name.value}</strong> has been successfully created`,
    }, 
    async () => {
      await scenariosApi.create({
        name: name.value,
        service: service.value ?? null,
        requestMethod: requestMethod.value ?? null,
        requestPath: requestPath.value ?? null,
        requestCondition: requestCondition.value ?? null,
        responseCode: responseCode.value ?? null,
        responseHeaders: responseHeaders.value && responseHeaders.value.length > 0 ? arrayToRecord(responseHeaders.value) : null,
        responseBody: null,
      });
      await scenariosStore.load();

      name.value = '';
      service.value = '';
      requestMethod.value = null;
      requestPath.value = null;
      requestCondition.value = null;
      responseCode.value = null;
      responseHeaders.value = null;
    },
  );
}

async function toggleValue(value: string) {
  
  switch(value) {
    case 'requestMethod':
      requestMethod.value = requestMethod.value === null ? '' : null;
      break;
    case 'requestPath':
      requestPath.value = requestPath.value === null ? '' : null;
      break;
    case 'requestCondition': 
      requestCondition.value = requestCondition.value === null ? '' : null;
      break;
    case 'responseCode':
      responseCode.value = responseCode.value === null ? 0 : null;
      break;
    case 'responseHeaders':
      responseHeaders.value = responseHeaders.value === null ? [{ header: '', value: '' }] : null;
      break;
    default:
      return;
  }
}
</script>

<template>
  <v-btn @click="uiStore.openDialog('scenario-add')" color="indigo">Add scenario</v-btn>

  <ModalWindow id="scenario-add" title="Add Scenario">
    
    <v-text-field v-model="name" label="Name" required></v-text-field>
    <v-select v-model="service" label="Service" :items="scenariosStore.list.map((s) => s.path)"></v-select>
    
    <span class="label">Request Conditions</span>
    <div class="row">
      <v-switch 
        :model-value="requestMethod !== null"
        color="indigo"
        :hide-details="true" 
        @update:model-value="toggleValue('requestMethod')">
      </v-switch>
      
      <v-select 
        label="Request Method" 
        v-if="requestMethod !== null"
        v-model="requestMethod" 
        :items="['GET', 'POST', 'PUT', 'PATCH', 'DELETE']"
        :hide-details="true">
      </v-select>
      <div class="fake-label" v-else>Request Method</div>
    </div>

    <div class="row">
      <v-switch 
        :model-value="requestPath !== null"
        color="indigo"
        :hide-details="true"
        @update:model-value="toggleValue('requestPath')">
      </v-switch>
      
      <v-text-field
        v-if="requestPath !== null" 
        v-model="requestPath" 
        label="Request Path" 
        :hide-details="true">
      </v-text-field>
      <div class="fake-label" v-else>Request Path</div>
    </div>

    <div class="row">
      <v-switch 
        :model-value="requestCondition !== null"
        color="indigo"
        :hide-details="true" 
        @update:model-value="toggleValue('requestCondition')">
      </v-switch>
      
      <v-text-field
        v-if="requestCondition !== null" 
        v-model="requestCondition" 
        label="Request Condition" 
        :hide-details="true">
      </v-text-field>
      <div class="fake-label" v-else>Request Condition</div>
    </div>

    <span class="label">Response Modifications</span>
    <div class="row">
      <v-switch 
        :model-value="responseCode !== null"
        color="indigo"
        :hide-details="true" 
        @update:model-value="toggleValue('responseCode')">
      </v-switch>
      
      <v-text-field
        label="Response Code" 
        v-if="responseCode !== null"
        v-model.number="responseCode" 
        :hide-details="true"
        type="number">
      </v-text-field>
      <div class="fake-label" v-else>Response Code</div>
    </div>
    
    <div class="row" id="responseHeaders">
      <v-switch 
        :model-value="responseHeaders && responseHeaders.length !== 0"
        @update:model-value="toggleValue('responseHeaders')"
        color="indigo"
        :hide-details="true" /> 
      <div class="fake-label">
        Response Headers
      </div>
    </div>

    <ResponseHeadersEdit
      v-if="responseHeaders && responseHeaders.length !== 0"
      v-model="responseHeaders"
    ></ResponseHeadersEdit>

    <div class="row" id="responseBody">

    </div>

    <template v-slot:actions>
      <v-btn @click="createScenario()" color="indigo">Create</v-btn>
      <v-btn @click="uiStore.closeDialog('scenario-add')">Close</v-btn>
    </template>
  </ModalWindow>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.label {
  font-weight: bold;
  margin: 1rem 0;
  display: block;
}

.fake-label {
  width: 100%;
  font-size: 16px;
  padding-left: 20px;
}
</style>
