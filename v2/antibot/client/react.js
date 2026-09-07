/**
 * Thin React helper for AntiBotClient (no React dependency — pass hooks in).
 *
 * @example
 * import { useAntiBot } from "./react.js";
 * const { issue, verify, loading } = useAntiBot({ React, client });
 */
export function createAntiBotHooks({ useState, useCallback, useRef }) {
  return function useAntiBot(client) {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const chRef = useRef(null);

    const issue = useCallback(
      async (params = {}) => {
        setLoading(true);
        setError(null);
        try {
          const ch = await client.issue(params);
          chRef.current = ch;
          return ch;
        } catch (e) {
          setError(e);
          throw e;
        } finally {
          setLoading(false);
        }
      },
      [client]
    );

    const verify = useCallback(
      async (answer, trajectory) => {
        setLoading(true);
        setError(null);
        try {
          const ch = chRef.current;
          if (!ch) throw new Error("antibot: call issue() first");
          return await client.verify(ch, answer, trajectory);
        } catch (e) {
          setError(e);
          throw e;
        } finally {
          setLoading(false);
        }
      },
      [client]
    );

    return { issue, verify, loading, error, challengeRef: chRef };
  };
}
