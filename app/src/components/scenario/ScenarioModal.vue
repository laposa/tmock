<script setup lang="ts">
import type { Scenario } from '@/apis/useScenariosApi';
import type { ResponseHeaderItem } from './headers/ResponseHeaderItem.vue';
import { isEqual } from 'lodash';

const props = defineProps<{
  detailLoading: boolean;
}>();

const scenariosStore = useScenariosStore();
const uiStore = useUiStore();
const scenariosApi = useScenariosApi();
const detail = computed(() => scenariosStore.detail);

const { snackbarWrapper } = useSnackbarWrapper();

watch(detail, (newDetail) => {
  if(newDetail) {
    id.value = newDetail.id;
    name.value = newDetail.name;
    service.value = newDetail.service;
    requestMethod.value = newDetail.requestMethod;
    requestPath.value = newDetail.requestPath;
    requestCondition.value = newDetail.requestCondition;
    responseCode.value = newDetail.responseCode;
    responseHeaders.value = newDetail.responseHeaders;
    responseHeadersArray.value = Object.entries(newDetail.responseHeaders ?? {}).map(([header, value]) => ({ header, value }));
    responseBody.value = newDetail.responseBody;
    responseBodyLanguage.value = getLanguageFromHeader(responseHeadersArray.value);
    skipProxy.value = newDetail.skipProxy;
  }
})

const id = ref(0);
const name = ref('');
const service = ref('');
const requestMethod = ref<string | null>(null);
const requestPath = ref<string | null>(null);
const requestCondition = ref<string | null>(null);
const responseCode = ref<number | null>(null);
const responseHeaders = ref<Record<string, string> | null>(null);
const responseHeadersArray = ref<ResponseHeaderItem[]>([]);
const responseBody = ref<string | null>(null);
const responseBodyLanguage = ref('Plain text');
const skipProxy = ref(false);

const confirmMessage = ref('');
const confirmAction = ref('');
const isEdit = computed (() => id.value !== 0);
const isLoading = ref(false);

function arrayToRecord(array: ResponseHeaderItem[]): Record<string, string> | null {
  if(array.length === 0) return null;

  return array.reduce((acc, { header, value }) => {
    acc[header] = value;
    return acc;
  }, {} as Record<string, string>);
}

function getLanguageFromHeader(array: ResponseHeaderItem[]): string {
  const contentType = array.find((item) => item.header === 'Content-Type')?.value;
  if (contentType?.includes('html')) return 'HTML';
  if (contentType?.includes('json')) return 'JSON';
  return 'Plain text';
}

function getScenarioDto() {
  return {
    name: name.value,
    service: service.value,
    requestMethod: requestMethod.value,
    requestPath: requestPath.value,
    requestCondition: requestCondition.value,
    responseCode: responseCode.value,
    responseHeaders: arrayToRecord(responseHeadersArray.value),
    responseBody: responseBody.value,
    skipProxy: skipProxy.value && responseBody.value !== null,
  }
}

async function createScenario() {
  snackbarWrapper(
    {
      errorTitle: `Failed to to add new scenario`,
      successMessage: `Scenario <strong>${name.value}</strong> has been successfully created`,
    }, 
    async () => {
      await scenariosApi.create(getScenarioDto());
      await scenariosStore.load();
      await uiStore.closeDialog('scenario-modal');
      resetValues();
    },
  );
}

async function saveScenario() {
  snackbarWrapper(
    {
      errorTitle: `Failed to save changes to the scenario ${name.value}`,
      successMessage: `Scenario <strong>${name.value}</strong> has been successfully updated`,
    }, 
    async () => {
      await scenariosApi.update(id.value.toString(), {
        id: id.value,
        ...getScenarioDto(),
      });
      await scenariosStore.load();
      await uiStore.closeDialog('scenario-modal');
      resetValues();
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
      responseHeaders.value = responseHeaders.value === null ? { header: '', value: '' } : null;
      responseHeadersArray.value = responseHeadersArray.value && responseHeadersArray.value.length > 0 ? [] : [{ header: '', value: '' }];
      break;
    case 'responseBody':
      responseBody.value = responseBody.value === null ? '' : null;
      break;
    case 'skipProxy':
      skipProxy.value = !skipProxy.value;
      break;
    default:
      return;
  }
}

async function confirm() {
  switch(confirmAction.value) {
    case 'close':
      uiStore.closeDialog('confirmation-dialog');
      uiStore.closeDialog('scenario-modal');
      break;
    case 'delete':
      uiStore.closeDialog('confirmation-dialog');
      snackbarWrapper(
        {
          errorTitle: `Failed to save changes to scenario ${name.value}`,
          successMessage: `Scenario <strong>${name.value}</strong> has been successfully deleted.`,
        },
        async () => {
          await scenariosStore.remove(scenariosStore.detail ? scenariosStore.detail.id.toString() : '');
          await uiStore.closeDialog('scenario-modal');
          await scenariosStore.load();
          resetValues();
        },
      );
      break;
    default:
      return;
  }
}

function cancel() {
  confirmAction.value = '';
  confirmMessage.value = '';
  isLoading.value = false;
  uiStore.closeDialog('confirmation-dialog');
}

function deleteScenario() {
  isLoading.value = true;
  confirmAction.value = 'delete';
  confirmMessage.value = 'Are you sure you want to delete this scenario?';
  uiStore.openDialog('confirmation-dialog');
}

function closeDialog() {
  const newScenario = {
    ...scenariosStore.detail,
    name: name.value,
    service: service.value,
    requestMethod: requestMethod.value,
    requestPath: requestPath.value,
    requestCondition: requestCondition.value,
    responseCode: responseCode.value,
    responseHeaders: arrayToRecord(responseHeadersArray.value),
  } as Scenario;

  if(!isEqual(newScenario, scenariosStore.detail)) {
    confirmAction.value = 'close';
    confirmMessage.value = 'You will lose unsaved changes. Are you sure you want to close?';
    uiStore.openDialog('confirmation-dialog');
  } else {
    uiStore.closeDialog('scenario-modal');
    isLoading.value = false;
    resetValues();
  }
}

function resetValues() {
  id.value = 0;
  name.value = '';
  service.value = '';
  requestMethod.value = null;
  requestPath.value = null;
  requestCondition.value = null;
  responseCode.value = null;
  responseHeaders.value = null;
  responseBody.value = null;
  responseHeadersArray.value = [];
  skipProxy.value = false;
}
</script>

<template>
  <ModalWindow 
    id="scenario-modal" 
    :title="isEdit ? 'Edit Scenario' : 'Add Scenario'" 
    :persistent="isEdit"
    :class="{ 'loading': props.detailLoading }">

    <!-- TODO find a way to make better transitions - could not access the modal height -->
    <div class="loading-block" v-if="props.detailLoading">
      <TmockLogoAnimated />
    </div>

    <div v-else>
      <v-text-field 
        v-model="name" 
        label="Name" 
        required>
      </v-text-field>

      <v-select 
        v-model="service" 
        :items="scenariosStore.list.map((s) => s.path)" 
        label="Service" 
        required>
      </v-select>

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
          :model-value="responseHeadersArray.length !== 0"
          @update:model-value="toggleValue('responseHeaders')"
          color="indigo"
          :hide-details="true" /> 
        <div class="fake-label">
          Response Headers
        </div>
      </div>

      <ResponseHeadersEdit
        v-if="responseHeadersArray.length !== 0"
        v-model="responseHeadersArray"
      ></ResponseHeadersEdit>

      <div id="responseBody" class="row response-body" :class="{ offset: responseHeadersArray.length !== 0}">
        <v-switch 
          :model-value="responseBody !== null"
          @update:model-value="toggleValue('responseBody')"
          color="indigo"
          :hide-details="true" /> 

          <CodeEditor 
            v-if="responseBody !== null"
            :title="'Response Body'"
            :language="responseBodyLanguage"
            v-model="responseBody">
          </CodeEditor>
          <div v-else class="fake-label">Response Body</div>
      </div>

      <div class="row" v-if="responseBody !== null">
        <v-switch 
          :model-value="skipProxy"
          @update:model-value="toggleValue('skipProxy')"
          color="indigo"
          :hide-details="true" /> 
        <div class="fake-label">
          Skip Proxy
        </div>
      </div>
    </div>

    <template v-slot:actions>
      <v-btn 
        v-if="isEdit"
        :loading="isLoading"
        :disabled="isLoading"
        color="red"
        @click="deleteScenario()">
          Delete
      </v-btn>

      <v-spacer></v-spacer>

      <v-btn 
        v-if="!isEdit" 
        :loading="isLoading"
        :disabled="isLoading || props.detailLoading"
        color="indigo"
        @click="createScenario()">
          Create
      </v-btn>

      <v-btn 
        v-if="isEdit" 
        :loading="isLoading"
        :disabled="isLoading || props.detailLoading"
        color="indigo"
        @click="saveScenario()">
          Save
      </v-btn>
      
      <v-btn 
        @click="closeDialog()">
          Close
      </v-btn>
    </template>

    <ConfirmationDialog 
      @confirm="confirm()"
      @discard="cancel()">
        {{ confirmMessage }}
    </ConfirmationDialog>
  </ModalWindow>
</template>

<style scoped>

.loading .v-card {
  max-height: 20px!important;
  background-color: yellow!important;
}

.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 15px;

  .v-switch {
    min-width: 40px;
  }
}

.response-body {
  align-items: unset;
  font-size: 16px; 

  .fake-label {
    padding-top: 15px;
  }
  
  :deep(.v-selection-control) {
    padding-top: 10px;
    align-items: flex-start;
  }
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

.offset {
  margin-top: 20px;
}

.loading .loading-block {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 500px;

  svg {
    width: 100px;
    max-width: 20%;
  }
}

</style>
