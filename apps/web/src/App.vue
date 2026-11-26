<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { alert, expired, signIn, signOut, useTask } from './api'
import type { Role, Session } from './api'
import AdminPage from './AdminPage.vue'
import MerchantPage from './MerchantPage.vue'
import ResidentPage from './ResidentPage.vue'
import LeaderPage from './LeaderPage.vue'

const navigation: Record<Role, [string, string][]> = {
  operator: [['pc/community', '社区与楼栋'], ['pc/partners', '商户与团长'], ['pc/catalog', '商品与库存']],
  merchant: [['pc/catalog', '商品与库存'], ['pc/orders', '即时单履约'], ['pc/campaigns', '团购与分拣'], ['pc/settlement', '佣金结算']],
  buyer: [['h5/nearby', '附近商户'], ['h5/groups', '社区团购'], ['h5/orders', '我的订单']],
  leader: [['h5/sorting', '分拣单'], ['h5/redeem', '提货核销'], ['h5/commission', '佣金明细']],
}
const roleNames: Record<Role, string> = { operator: '平台运营', merchant: '商户管理', buyer: '社区居民', leader: '自提团长' }

const session = ref<Session>()
const credential = ref('')
const route = ref(location.hash.slice(2))
const dialog = ref<HTMLDialogElement>()
const { busy, run } = useTask()
const tabs = computed(() => session.value ? navigation[session.value.role] : [])
const current = computed(() => tabs.value.find(tab => tab[0] === route.value) ?? tabs.value[0])
const mobile = computed(() => current.value?.[0].startsWith('h5/'))
const residentArea = ref({ community: '', circle: '', lastOrder: '' })
const page = computed(() => current.value?.[0].split('/')[1] ?? '')
const adminPage = computed(() => ['community', 'partners', 'catalog'].includes(page.value))

function readRoute() {
  route.value = location.hash.slice(2)
  window.scrollTo(0, 0)
}
onMounted(() => window.addEventListener('hashchange', readRoute))
onUnmounted(() => window.removeEventListener('hashchange', readRoute))

watch(alert, (value) => {
  if (value && !dialog.value?.open) dialog.value?.showModal()
})
watch(expired, (value) => {
  if (value) {
    leave()
    expired.value = false
  }
})
watch(current, (value) => {
  if (value && value[0] !== route.value) location.replace('#/' + value[0])
})

function leave() {
  signOut()
  session.value = undefined
  residentArea.value = { community: '', circle: '', lastOrder: '' }
  credential.value = ''
}
function login() {
  void run(async () => {
    session.value = await signIn(credential.value.trim())
    credential.value = ''
  })
}
</script>

<template>
  <main v-if="!session" class="signin">
    <section class="signin-intro">
      <span class="brand"><span class="brand-mark">半</span> 半公里</span>
      <h1>社区配送<br>与团购</h1>
    </section>
    <section class="signin-box">
      <form class="card login-card" @submit.prevent="login">
        <h2>进入半公里</h2>
        <p>使用管理员发放的访问令牌登录，系统会根据身份打开对应工作区。</p>
        <label>访问令牌<input v-model="credential" type="password" required minlength="32" maxlength="256" autocomplete="off"></label>
        <button :disabled="busy" type="submit">{{ busy ? '正在登录…' : '登录' }}</button>
      </form>
    </section>
  </main>
  <div v-else-if="current" :class="mobile ? 'shell mobile' : 'shell desktop'">
    <header class="topbar">
      <span class="brand"><span class="brand-mark">半</span> 半公里</span>
      <div class="account"><span>{{ roleNames[session.role] }}</span><button class="secondary" type="button" @click="leave">退出</button></div>
    </header>
    <nav aria-label="业务导航">
      <a v-for="tab in tabs" :key="tab[0]" :href="'#/' + tab[0]" :aria-current="current[0] === tab[0] ? 'page' : undefined">{{ tab[1] }}</a>
    </nav>
    <main class="workspace">
      <h1>{{ current[1] }}</h1>
      <AdminPage v-if="adminPage" :key="page" :page="page" :session="session" />
      <MerchantPage v-else-if="session.role === 'merchant'" :key="page" :page="page" :store="session.id" />
      <ResidentPage v-else-if="session.role === 'buyer'" :key="page" v-model:area="residentArea" :page="page" />
      <LeaderPage v-else :key="page" :page="page" />
    </main>
  </div>
  <dialog ref="dialog" aria-labelledby="error-title" @close="alert = ''">
    <h2 id="error-title">操作未完成</h2>
    <p>{{ alert }}</p>
    <form method="dialog"><button autofocus>确定</button></form>
  </dialog>
</template>
