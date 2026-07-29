<template>
  <div class="route-skeleton" aria-label="页面加载中" aria-busy="true">
    <aside>
      <n-skeleton height="38px" width="132px" />
      <n-skeleton v-for="index in 8" :key="index" height="40px" />
    </aside>
    <main>
      <n-skeleton text width="96px" />
      <n-skeleton height="42px" width="240px" />
      <n-skeleton height="118px" />
      <section>
        <n-skeleton v-for="index in 3" :key="index" height="88px" />
      </section>
      <n-skeleton height="260px" />
    </main>
  </div>
</template>

<style scoped>
.route-skeleton { display: grid; grid-template-columns: 228px minmax(0, 1fr); min-height: 100dvh; background: #eef4f1; }
aside { display: grid; align-content: start; gap: 10px; padding: 24px 18px; border-right: 1px solid rgba(38,74,61,.12); background: rgba(255,255,255,.86); }
main { display: grid; align-content: start; gap: 16px; width: min(100%, 1420px); padding: 38px clamp(20px,4vw,52px); }
section { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 12px; }
@media(max-width:860px){.route-skeleton{grid-template-columns:1fr}aside{display:none}main{padding:22px 14px 80px}section{grid-template-columns:1fr}}
</style>
