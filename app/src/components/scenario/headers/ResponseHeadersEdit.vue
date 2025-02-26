<script setup lang="ts">
import type { ResponseHeaderItem } from './ResponseHeaderItem.vue';

const emit = defineEmits<{
  'update': [Record<string, string>],
}>();

const props = defineProps<{
  headers: Record<string, string> | null,
}>();

let headersArr = props.headers ? Object.entries(props.headers).map(([header, value]) => ({ header, value })) : [];

function update() {
	emit('update', arrayToRecord(headersArr));
}

function arrayToRecord(array: ResponseHeaderItem[]): Record<string, string> {
  return array.reduce((acc, { header, value }) => {
    acc[header] = value;
    return acc;
  }, {} as Record<string, string>);
}

function deleteResponseHeader(header: ResponseHeaderItem) {
	headersArr = headersArr.filter((item) => item.header !== header.header);
}

</script>

<template>
	<div class="headers">
		<div class="row" v-for="header in headersArr" :key="header.value">
			<ResponseHeaderItem 
				:header="header"
				@delete="deleteResponseHeader(header)"
			/>
		</div>
		<v-btn 
			prepend-icon="mdi-plus"
			@click="headersArr.push({ header: '', value: '' })"> 
			Add header
		</v-btn>
		<v-btn 
			prepend-icon="mdi-check"
			@click="update()">
			Save
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
