<script setup lang="ts">
import type { ResponseHeaderItem } from './ResponseHeaderItem.vue';

export type ResponseHeaders = Record<string, string>;

const emit = defineEmits<{
	'update:model-value': [ResponseHeaders],
}>();	

const headers = defineModel<ResponseHeaders>({ required: true });

function updateHeader(headerToUpdate: string, { header, value }: ResponseHeaderItem) {
	const newHeaders = { ...headers.value };

	if (headerToUpdate !== header) {
		delete newHeaders[headerToUpdate];
	}

	console.log({ header, value });

	emit('update:model-value', { ...newHeaders, [header]: value });
}

function removeHeader(header: string) {
	// eslint-disable-next-line @typescript-eslint/no-unused-vars
	const { [header]: _, ...rest } = headers.value;
	emit('update:model-value', rest);
}

function addHeader() {
	emit('update:model-value', { ...headers.value, '': '' });
}
</script>

<template>
	<div class="headers">
		<div class="row" v-for="([header, value], index) in Object.entries(headers)" :key="index">
			<ResponseHeaderItem 
				:model-value="{ header, value }"
				@update:model-value="(val) => updateHeader(header, val)"
				@delete="removeHeader(header)"
			/>
		</div>
		<v-btn 
			prepend-icon="mdi-plus"
			@click="addHeader"> 
			Add header
		</v-btn>
	</div>
</template>

<style scoped>
.headers {
	padding-left: 55px;

	.row:last-of-type {
		margin-bottom: 10px;
	}
}
</style>
