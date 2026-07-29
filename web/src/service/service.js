import { useFetch } from "@vueuse/core";
import { localizeResponseMessages } from "@/utils/apiError";

const REQUEST_TIMEOUT_MS = 8000;

const publishConnectionState = (detail) => {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent("pst-connection-state", { detail }));
  }
};

class Service {
  /**
   * Fetches data from a specified URL.
   *
   * @param {string} url - The URL to fetch data from.
   * @return {Promise<Response>} A Promise that resolves to the response from the server.
   */
  fetch(url) {
    return useFetch(`${url}`, {
      updateDataOnError: true,
      timeout: REQUEST_TIMEOUT_MS,
      beforeFetch({ options }) {
        const token = localStorage.getItem("palworld_token");
        options.headers = {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
          ...options.headers,
          "Remote-Ip-Address": localStorage.getItem("ip") || "127.0.0.1",
        };
        return {
          options,
        };
      },
      afterFetch(context) {
        context.data = localizeResponseMessages(
          context.data,
          !context.response?.ok,
        );
        publishConnectionState({ state: "online", message: "" });
        return context;
      },
      onFetchError(context) {
        const status = context.response?.status;
        if (status === 401) {
          localStorage.removeItem("palworld_token");
        }
        if (!context.response) {
          const timedOut = /timeout|aborted/i.test(String(context.error || ""));
          context.data = {
            error: timedOut
              ? "连接 PST 超过 8 秒没有响应"
              : "无法连接 PST",
          };
          publishConnectionState({
            state: timedOut ? "timeout" : "offline",
            message: timedOut
              ? "PST 超过 8 秒没有响应"
              : "当前无法连接 PST",
          });
        }
        return context;
      },
    });
  }

  /**
   * Generates a query string from a given credential object.
   *
   * @param {Object} credential - The credential object.
   * @return {string} - The generated query string.
   */
  generateQuery(credential) {
    const entries = Object.entries(credential);
    return entries
      .reduce((accumulation, [key, value]) => {
        if (value !== undefined && value !== null && value !== "") {
          accumulation.push(
            `${encodeURIComponent(key)}=${encodeURIComponent(value)}`,
          );
        }
        return accumulation;
      }, [])
      .join("&");
  }
}

export default Service;
