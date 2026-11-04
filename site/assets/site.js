(function(){try{var els=document.querySelectorAll('.reveal');if(!('IntersectionObserver' in window)){els.forEach(function(e){e.classList.add('in')});return}
var io=new IntersectionObserver(function(es){es.forEach(function(e){if(e.isIntersecting){e.target.classList.add('in');io.unobserve(e.target)}})},{threshold:.12});els.forEach(function(e){io.observe(e)})}catch(e){document.querySelectorAll('.reveal').forEach(function(x){x.classList.add('in')})}})();

(function(){var n=document.querySelector('nav.top');if(!n)return;var on=function(){n.classList.toggle('scrolled',window.scrollY>8)};on();window.addEventListener('scroll',on,{passive:true})})();
