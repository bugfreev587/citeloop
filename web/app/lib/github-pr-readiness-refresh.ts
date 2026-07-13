export type GithubPRReadinessRefreshMode = "normal" | "after-mutation";

type GenerationPromise<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
};

export function createGithubPRReadinessRefreshCoordinator<T>(
  execute: () => Promise<T>,
  onDrainingChange?: (draining: boolean) => void,
) {
  let requestedGeneration = 0;
  let completedGeneration = 0;
  let runningGeneration = 0;
  let draining = false;
  const generationPromises = new Map<number, GenerationPromise<T>>();

  function promiseFor(generation: number): GenerationPromise<T> {
    const existing = generationPromises.get(generation);
    if (existing) return existing;

    let resolve!: (value: T) => void;
    let reject!: (error: unknown) => void;
    const promise = new Promise<T>((nextResolve, nextReject) => {
      resolve = nextResolve;
      reject = nextReject;
    });
    const created = { promise, resolve, reject };
    generationPromises.set(generation, created);
    return created;
  }

  async function drain() {
    try {
      while (completedGeneration < requestedGeneration) {
        const generation = completedGeneration + 1;
        const pending = promiseFor(generation);
        runningGeneration = generation;
        try {
          const result = await execute();
          completedGeneration = generation;
          pending.resolve(result);
        } catch (error) {
          completedGeneration = generation;
          pending.reject(error);
        } finally {
          runningGeneration = 0;
          generationPromises.delete(generation);
        }
      }
    } finally {
      draining = false;
      onDrainingChange?.(false);
      if (completedGeneration < requestedGeneration) startDrain();
    }
  }

  function startDrain() {
    if (draining) return;
    draining = true;
    onDrainingChange?.(true);
    void drain();
  }

  function request(mode: GithubPRReadinessRefreshMode = "normal"): Promise<T> {
    let targetGeneration: number;
    if (mode === "after-mutation") {
      targetGeneration = runningGeneration
        ? runningGeneration + 1
        : requestedGeneration > completedGeneration
          ? requestedGeneration
          : completedGeneration + 1;
    } else {
      targetGeneration = runningGeneration || (requestedGeneration > completedGeneration ? requestedGeneration : completedGeneration + 1);
    }

    requestedGeneration = Math.max(requestedGeneration, targetGeneration);
    const pending = promiseFor(targetGeneration).promise;
    startDrain();
    return pending;
  }

  return { request };
}
