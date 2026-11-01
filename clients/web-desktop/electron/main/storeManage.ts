import Store from "electron-store";

let store: Store;

export const getStore = () => {
  if (!store) {
    store = new Store(process.env.OPENIM38_TEST_DATA_DIR ? {cwd:process.env.OPENIM38_TEST_DATA_DIR} : {});
  }
  return store;
};

export { Store };
