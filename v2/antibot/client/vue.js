/**
 * Thin Vue 3 composable factory for AntiBotClient (no Vue dependency — pass ref/shallowRef).
 *
 * @example
 * import { useAntiBot } from "./vue.js";
 * const ab = useAntiBot({ ref, client });
 */
export function createAntiBotComposable({ ref }) {
  return function useAntiBot(client) {
    const loading = ref(false);
    const error = ref(null);
    const challenge = ref(null);

    async function issue(params = {}) {
      loading.value = true;
      error.value = null;
      try {
        challenge.value = await client.issue(params);
        return challenge.value;
      } catch (e) {
        error.value = e;
        throw e;
      } finally {
        loading.value = false;
      }
    }

    async function runPrecheck(opts = {}) {
      loading.value = true;
      error.value = null;
      try {
        const ch = await client.runPrecheck(opts);
        if (ch?.id) challenge.value = ch;
        return ch;
      } catch (e) {
        error.value = e;
        throw e;
      } finally {
        loading.value = false;
      }
    }

    async function verify(answer, trajectory) {
      loading.value = true;
      error.value = null;
      try {
        if (!challenge.value) throw new Error("antibot: call issue() or runPrecheck() first");
        return await client.verify(challenge.value, answer, trajectory);
      } catch (e) {
        error.value = e;
        throw e;
      } finally {
        loading.value = false;
      }
    }

    return { issue, runPrecheck, verify, loading, error, challenge };
  };
}
