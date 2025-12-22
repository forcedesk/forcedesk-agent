<template>
    <div class="min-h-screen bg-gradient-to-br from-gray-900 via-gray-800 to-gray-900 flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-md w-full">
            <!-- Login Card -->
            <div class="bg-gray-800 rounded-2xl shadow-2xl border border-gray-700 overflow-hidden">
                <!-- Header with gradient -->
                <div class="bg-gradient-to-r from-indigo-600 to-blue-600 px-8 pt-8 pb-6">
                    <div class="text-center">
                        <div class="inline-flex items-center justify-center w-20 h-20 bg-gray-900 rounded-2xl shadow-lg mb-4">
                            <img
                                src="https://cdn.forcedesk.io/img/forcedesk-icon-test.svg"
                                alt="ForceDesk Logo"
                                class="h-12 w-12"
                            />
                        </div>
                        <h2 class="text-3xl font-bold text-white">
                            ForceDesk Agent
                        </h2>
                        <p class="mt-2 text-sm text-indigo-100">
                            Configuration Panel
                        </p>
                    </div>
                </div>

                <!-- Form Section -->
                <div class="px-8 py-8">
                    <form @submit.prevent="handleLogin" class="space-y-6">
                        <!-- Password Input with Icon -->
                        <div>
                            <label for="password" class="block text-sm font-medium text-gray-300 mb-2">
                                Admin Password
                            </label>
                            <div class="relative">
                                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                                    <Lock class="h-5 w-5 text-gray-500" />
                                </div>
                                <input
                                    id="password"
                                    v-model="form.password"
                                    type="password"
                                    required
                                    :class="[
                                        'block w-full pl-10 pr-10 py-3 border rounded-lg focus:outline-none focus:ring-2 sm:text-sm transition-colors bg-gray-700 text-white',
                                        errors.password
                                            ? 'border-red-500 placeholder-red-400 focus:border-red-400 focus:ring-red-400'
                                            : 'border-gray-600 placeholder-gray-400 focus:border-indigo-500 focus:ring-indigo-500'
                                    ]"
                                    placeholder="Enter your password"
                                    autocomplete="current-password"
                                />
                                <div v-if="errors.password" class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                                    <AlertCircle class="h-5 w-5 text-red-400" />
                                </div>
                            </div>
                        </div>

                        <!-- Error Message with Animation -->
                        <transition
                            enter-active-class="transition ease-out duration-200"
                            enter-from-class="opacity-0 transform scale-95"
                            enter-to-class="opacity-100 transform scale-100"
                            leave-active-class="transition ease-in duration-150"
                            leave-from-class="opacity-100 transform scale-100"
                            leave-to-class="opacity-0 transform scale-95"
                        >
                            <div v-if="errors.password" class="rounded-lg bg-red-900/30 border border-red-500/50 p-4">
                                <div class="flex items-start">
                                    <div class="flex-shrink-0">
                                        <AlertTriangle class="h-5 w-5 text-red-400" />
                                    </div>
                                    <div class="ml-3">
                                        <h3 class="text-sm font-medium text-red-300">
                                            Authentication Failed
                                        </h3>
                                        <p class="mt-1 text-sm text-red-400">
                                            {{ errors.password }}
                                        </p>
                                    </div>
                                </div>
                            </div>
                        </transition>

                        <!-- Submit Button -->
                        <div>
                            <button
                                type="submit"
                                :disabled="loading"
                                class="group relative w-full flex justify-center items-center gap-2 py-3 px-4 border border-transparent text-sm font-medium rounded-lg text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 shadow-md hover:shadow-lg"
                            >
                                <Loader2 v-if="loading" class="h-5 w-5 animate-spin" />
                                <LogIn v-else class="h-5 w-5" />
                                {{ loading ? 'Signing in...' : 'Sign in' }}
                            </button>
                        </div>
                    </form>

                    <!-- Additional Info -->
                    <div class="mt-6 text-center">
                        <p class="text-xs text-gray-400">
                            Secured access to agent configuration
                        </p>
                    </div>
                </div>
            </div>

            <!-- Footer -->
            <div class="mt-8 text-center">
                <p class="text-sm text-gray-400">
                    Powered by <span class="font-semibold text-indigo-400">ForceDesk</span>
                </p>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref } from 'vue';
import { router } from '@inertiajs/vue3';
import { Lock, LogIn, Loader2, AlertCircle, AlertTriangle } from 'lucide-vue-next';

const props = defineProps({
    errors: {
        type: Object,
        default: () => ({}),
    },
});

const form = ref({
    password: '',
});

const loading = ref(false);

function handleLogin() {
    loading.value = true;

    router.post('/login', form.value, {
        onError: (errors) => {
            // Errors are automatically handled by Inertia and passed to the component
            loading.value = false;
        },
        onFinish: () => {
            loading.value = false;
        },
        preserveScroll: true,
    });
}
</script>
