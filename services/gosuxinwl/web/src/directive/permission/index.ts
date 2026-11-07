import { DirectiveBinding } from 'vue';
import { useUserStore } from '@/store';
function checkPermission(el: HTMLElement, binding: DirectiveBinding) {
  const { value } = binding;
  const userStore = useUserStore();
  const { btnroles } = userStore;
 if (Array.isArray(value)&&value.length > 0) {
     if (btnroles){
        let hasPermission=false
        value.forEach((item)=>{
            hasPermission = btnroles.includes(item);
        })
        const Allrule = btnroles?.includes("*");
        if (!hasPermission && el.parentNode&&!Allrule) {
          el.parentNode.removeChild(el);
        }
     }
  } else {
    throw new Error(`need btnroles! Like v-permission="['add']"`);
  }
}

export default {
  mounted(el: HTMLElement, binding: DirectiveBinding) {
    checkPermission(el, binding);
  },
  updated(el: HTMLElement, binding: DirectiveBinding) {
    checkPermission(el, binding);
  },
};
