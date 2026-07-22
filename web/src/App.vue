<script setup>
import { zhCN, dateZhCN, darkTheme } from "naive-ui";
import pageStore from "@/stores/model/page.js";
import { onBeforeUnmount, onMounted, ref } from "vue";

localStorage.setItem("locale", "zh");
const isDarkMode = ref(window.matchMedia("(prefers-color-scheme: dark)").matches);
const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
const updateDarkMode = (event) => { isDarkMode.value = event.matches; };
const updateScreenWidth = () => pageStore().setScreenWidth(document.documentElement.clientWidth || window.innerWidth);
const themeOverrides = {
  common: {
    primaryColor: "#2f7d68",
    primaryColorHover: "#3d9279",
    primaryColorPressed: "#276957",
    primaryColorSuppl: "#3d9279",
    borderRadius: "9px",
  },
};

onMounted(() => {
  mediaQuery.addEventListener("change", updateDarkMode);
  window.addEventListener("resize", updateScreenWidth, { passive: true });
  updateScreenWidth();
});
onBeforeUnmount(() => {
  mediaQuery.removeEventListener("change", updateDarkMode);
  window.removeEventListener("resize", updateScreenWidth);
});
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN" :theme-overrides="themeOverrides" :theme="isDarkMode ? darkTheme : null">
    <n-dialog-provider>
      <n-notification-provider>
        <n-message-provider><router-view /></n-message-provider>
      </n-notification-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>
