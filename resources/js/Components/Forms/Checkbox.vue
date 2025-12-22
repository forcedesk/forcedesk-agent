<template>
    <div class="flex items-start">
        <div class="flex items-center h-5">
            <input
                :id="id"
                type="checkbox"
                :checked="modelValue"
                @change="$emit('update:modelValue', $event.target.checked)"
                :disabled="disabled"
                :required="required"
                :class="[
                    'h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-600 rounded bg-gray-700',
                    disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
                    inputClass
                ]"
            />
        </div>
        <div class="ml-3">
            <label v-if="label" :for="id" :class="[
                'text-sm font-medium text-gray-300',
                disabled ? 'opacity-50' : 'cursor-pointer'
            ]">
                {{ label }}
                <span v-if="required" class="text-red-400 ml-1">*</span>
            </label>
            <p v-if="description" class="text-xs text-gray-500 dark:text-gray-400">
                {{ description }}
            </p>
            <p v-if="error" class="mt-1 text-xs text-red-400">{{ error }}</p>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
    modelValue: {
        type: Boolean,
        default: false,
    },
    label: {
        type: String,
        default: null,
    },
    description: {
        type: String,
        default: null,
    },
    required: {
        type: Boolean,
        default: false,
    },
    disabled: {
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
    return `checkbox-${Math.random().toString(36).substr(2, 9)}`;
});
</script>
