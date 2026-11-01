const thirdRoutes = [
  {
    path: "moments",
    async lazy() {
      const { Moments } = await import("@/pages/third/moments");
      return { Component: Moments };
    },
  },
  {
    path: "personal-settings",
    async lazy() {
      const { PersonalSettings } = await import("@/pages/third/personalSettings");
      return { Component: PersonalSettings };
    },
  },
  {
    path: "about",
    async lazy() {
      const { About } = await import("@/pages/third/about");
      return { Component: About };
    },
  },
  {
    path: "black-list",
    async lazy() {
      const { BlackList } = await import("@/pages/third/blackList");
      return { Component: BlackList };
    },
  },
  {
    path: "change-password",
    async lazy() {
      const { ChangePassword } = await import("@/pages/third/changePassword");
      return { Component: ChangePassword };
    },
  },
  {
    path: "choose-contact",
    async lazy() {
      const { ChooseContact } = await import("@/pages/third/chooseContact");
      return { Component: ChooseContact };
    },
  },
];

export default thirdRoutes;
