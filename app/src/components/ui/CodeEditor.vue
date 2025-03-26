<script setup lang="ts">
import { html } from '@codemirror/lang-html';
import { json } from '@codemirror/lang-json';
import type { Extension } from '@codemirror/state';
import type { Ref } from 'vue';

const content = defineModel<string | null>();
const props = defineProps<{
  title: string;
  language: string;
}>();

const languageMap: Record<string, () => Extension> = {
  'HTML': html,
  'JSON': json,
};

const isLoading = ref(false);
const contentCode = ref(content);
const language = ref(props.language);
const extensions: Ref<Extension[]> = ref([]);
extensions.value = props.language == 'Plain text' ? [] : [languageMap[props.language]()];

function changeLanguage(lang: string) {
  isLoading.value = true;
  if (lang === 'Plain text') {
    extensions.value = [];
  } else {
    const langFunction = languageMap[lang];
    if (langFunction) {
      extensions.value = [langFunction()];
    }
  }
  language.value = lang;
  isLoading.value = false;
}
</script>

<template>
  <div class="code-editor">
    <div class="toolbar">
      <slot default>
        {{ props.title }}
        <div class="select-wrapper">
          <v-select
            variant="solo"
            :model-value="language"
            :items="['JSON', 'HTML', 'Plain text']"
            :hide-details="true"
            density="compact"
            @update:model-value="(value) => changeLanguage(value)"
          ></v-select>
        </div>
      </slot>
    </div>

    <codemirror 
      v-if="!isLoading"
      :extensions="extensions"
      v-model="contentCode">
    </codemirror> 
  </div>
</template>

<style scoped>
  .v-codemirror {
    width: 100%;
  }

  .code-editor {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    width: 100%;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 20px;
    width: 100%;
    border-top-left-radius: 5px;
    border-top-right-radius: 5px;
    background-color: #ddd;
  }

  .language-select {
    display: flex;
    align-items: center;
    padding: 5px;
    outline: none; 
    text-align: right;
    position: relative;

    &:before {
      content: 'a';
      position: relative;
    }
  }

  .select-wrapper {
    max-width: 150px;
  }

  :deep(.cm-editor) {
    width: 100%;
    min-height: 100px;
    border: 1px solid #ddd;
    outline: none !important;
  }

  :deep(.v-select) {
    .v-field {
      width: 200px;
    }
  }
</style>
