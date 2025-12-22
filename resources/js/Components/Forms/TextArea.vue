<template>
    <div>
        <label v-if="label" :for="id" class="block text-base font-medium text-gray-300 mb-2">
            {{ label }}
            <span v-if="required" class="text-red-400 ml-1">*</span>
        </label>
        <p v-if="description" class="mt-1 text-sm text-gray-500 dark:text-gray-400 mb-2">
            {{ description }}
        </p>
        <textarea
            :id="id"
            :value="modelValue"
            @input="$emit('update:modelValue', $event.target.value)"
            :placeholder="placeholder"
            :required="required"
            :disabled="disabled"
            :readonly="readonly"
            :rows="rows"
            :class="[
                'shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full px-4 py-3 text-base border-gray-600 rounded-lg bg-gray-700 text-white placeholder-gray-400',
                error ? 'border-red-500 focus:border-red-500 focus:ring-red-500' : '',
                disabled ? 'opacity-50 cursor-not-allowed' : '',
                inputClass
            ]"
        ></textarea>
        <p v-if="error" class="mt-1 text-xs text-red-400">{{ error }}</p>
    </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
    modelValue: {
        type: [String, Number],
        default: '',
    },
    label: {
        type: String,
        default: null,
    },
    description: {
        type: String,
        default: null,
    },
    placeholder: {
        type: String,
        default: '',
    },
    rows: {
        type: Number,
        default: 3,
    },
    required: {
        type: Boolean,
        default: false,
    },
    disabled: {
        type: Boolean,
        default: false,
    },
    readonly: {
        type: Boolean,
        default: false,
    },
    error: {
        type: String,
        default: null,
    },
    inputClass: {
        type: String,
        default: '',
    },
});

const emit = defineEmits(['update:modelValue']);

const id = computed(() => {
    return `textarea-${Math.random().toString(36).substr(2, 9)}`;
});
</script>
