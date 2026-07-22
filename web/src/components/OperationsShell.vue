<script setup>
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";

defineProps({ title: String, subtitle: String });
const router = useRouter();
const { t } = useI18n();
const isAdmin = computed(() => Boolean(localStorage.getItem("palworld_token")));
</script>

<template>
  <div class="operations-shell">
    <header class="operations-header">
      <div>
        <n-button text type="primary" @click="router.push('/')">← {{ t("enhanced.home") }}</n-button>
        <h1>{{ title }}</h1>
        <p>{{ subtitle }}</p>
      </div>
      <n-space class="operations-nav">
        <n-button secondary @click="router.push('/work-pals')">{{ t("enhanced.workPals") }}</n-button>
        <n-button v-if="isAdmin" secondary @click="router.push('/inventory')">{{ t("enhanced.inventory") }}</n-button>
        <n-button v-if="isAdmin" secondary @click="router.push('/breeding-farms')">{{ t("enhanced.breedingFarms") }}</n-button>
        <n-button v-if="isAdmin" secondary @click="router.push('/world-settings')">{{ t("enhanced.worldSettings") }}</n-button>
      </n-space>
    </header>
    <main class="operations-content"><slot /></main>
  </div>
</template>

<style scoped>
.operations-shell { min-height: 100vh; background: var(--n-color, #f5f7fb); }
.operations-header { display:flex; align-items:flex-end; justify-content:space-between; gap:24px; padding:24px clamp(16px,4vw,52px); border-bottom:1px solid rgba(128,128,128,.18); background:var(--n-color, #fff); }
h1 { margin:8px 0 2px; font-size:clamp(24px,3vw,36px); line-height:1.1; }
p { margin:0; opacity:.62; }
.operations-content { max-width:1480px; margin:0 auto; padding:24px clamp(12px,3vw,36px) 56px; }
@media (max-width: 767px) { .operations-header { align-items:flex-start; flex-direction:column; padding:16px; } .operations-nav { width:100%; overflow-x:auto; flex-wrap:nowrap!important; } .operations-content { padding:14px 12px 36px; } }
</style>
