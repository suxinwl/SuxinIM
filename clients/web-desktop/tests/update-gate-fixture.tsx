import React from "react";
import {createRoot} from "react-dom/client";
import {useAutoUpdate} from "../src/layout/useAutoUpdate";
function Fixture(){return useAutoUpdate() ?? <div data-testid="business">Business fixture</div>;}
createRoot(document.getElementById("root")!).render(<Fixture />);
