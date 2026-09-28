# Подключение новых платёжных сервисов

В **Админка → Интеграции** раскройте карточку сервиса, заполните реквизиты, включите интеграцию и сохраните. Ключи хранятся в зашифрованном виде в базе; добавлять их в `.env` не нужно.

После сохранения в карточке появится **Webhook URL**. Укажите его в кабинете платёжного сервиса, если это требуется в таблице. Входящие уведомления должны быть доступны по HTTPS. Сначала проверьте оплату в тестовом режиме своего сервиса: автотесты Link-Bot проверяют протокол и подписи на имитации API, реальные мерчантские ключи в них не используются.

| Сервис | Поля в Link-Bot | Настройка уведомлений | Официальная документация |
| --- | --- | --- | --- |
| anore.cc | Shop ID, API-ключ, Secret Key | Адрес передаётся при создании платежа как `callbackUrl`; Secret Key нужен для подписи уведомления | [anore.cc](https://anore.cc/docs) |
| MulenPay | Shop ID, API-ключ, секрет подписи из личного кабинета | Вставьте Webhook URL в настройку callback магазина | [MulenPay](https://docs.mulenpay.com/) |
| AuraPay | ID кассы, API-ключ, секретный ключ №2 | Адрес передаётся при создании счёта как `callback_url` | [AuraPay](https://docs.aurapay.tech/) |
| AntiloPay | Идентификатор проекта, Secret ID мерчанта, приватный ключ Base64, публичный ключ Callback Base64 | Вставьте Webhook URL в настройки уведомлений проекта | [AntiloPay API PDF](https://doc.antilopay.com/AntilopayAPI.pdf) |
| ParityPay | ID кассы, секретный ключ №1, секретный ключ №2 | Адрес передаётся при создании счёта как `callback_url` | [ParityPay v2](https://docs.paritypay.net/) |
| Tribute | API-ключ, Shop ID при нескольких магазинах | Вставьте Webhook URL в `callbackUrl` своего магазина | [Tribute Shop API](https://wiki.tribute.tg/for-shops/api/methods) |
| CloudPayments | Public ID и API Secret | Настройте уведомление **Pay**, метод **POST**, формат **CloudPayments**, URL из Link-Bot | [CloudPayments](https://developers.cloudpayments.ru/) |

AntiloPay использует два разных ключа: приватный ключ мерчанта для исходящих запросов и публичный ключ проекта для Callback. Вставляйте Base64-значение, выданное кабинетом, без заголовков PEM. Приватный ключ должен быть RSA PKCS8, публичный — RSA SubjectPublicKeyInfo. Email покупателя берётся из подтверждённого Google-профиля или запрашивается в Mini App перед оплатой. IP передаётся из запроса Mini App.

Tribute подключается через **Shop API**, с разовыми заказами `period=onetime`. Нужен магазин, принимающий обычные платежи: магазин только для Stars не возвращает браузерную ссылку. Прежняя интеграция Tribute для подписок Telegram продолжает использовать свой отдельный тип платежа.

Описание заказа включает срок, устройства и трафик из тарифа/пакета. Успешное уведомление проходит проверку подписи, номера заказа, суммы и валюты. Для MulenPay, где подпись callback не описана, Link-Bot дополнительно запрашивает реальный статус платежа через авторизованный API. Повторный вебхук не начисляет покупку повторно.

## Сервисы, для которых нужна документация

В каталоге присутствуют [Datagio](https://datagio.finance/), [Paycore](https://paycore.pw/) и [Kassa AI](https://kassa.ai/), но включить их нельзя. На момент добавления доступную официальную документацию создания платежей и проверки уведомлений найти не удалось; ссылки Datagio на документацию отвечали 404. Paycore выбран по сайту `paycore.pw`; у других сервисов с таким названием API может отличаться.

Для продолжения нужны документация от поддержки сервиса или доступная ссылка на неё: создание счёта, авторизация, формат уведомления, подпись/проверка статуса и ответ на вебхук. До реализации такие сервисы не показываются покупателю среди способов оплаты.

## Официальные логотипы

Файлы `internal/miniapp/static/assets/payment-*.png` имеют размер 512×512. Логотипы скачаны с сайтов сервисов; SVG преобразованы в PNG, для некоторых выделен фирменный знак из полного логотипа. Для тёмных надписей сохранён белый фон. Новые логотипы не рисовались и не генерировались.

| PNG | Официальный исходник |
| --- | --- |
| `payment-datagio.png` | [PNG 512×512](https://datagio.finance/android-chrome-512x512.png) |
| `payment-paycore.png` | [SVG](https://paycore.pw/img/pc-logo-text-bottom.214db149.svg) |
| `payment-mulenpay.png` | [SVG](https://mulenpay.com/favicon.svg) |
| `payment-anore.png` | [SVG](https://anore.cc/images/anore-logo.svg) |
| `payment-aurapay.png` | [SVG](https://aurapay.tech/images/logo.svg) |
| `payment-antilopay.png` | [SVG с сайта AntiloPay](https://static.tildacdn.com/tild6661-3532-4138-b334-306134636237/antilopay_logo.svg) |
| `payment-paritypay.png` | [PNG](https://paritypay.net/upload/iblock/3ef/3oxj3264ncnfelp3v31hitg7lp9rsrda/logo.png) |
| `payment-tribute.png` | [Логотип в официальной документации Tribute](https://wiki.tribute.tg/) |
| `payment-kassaai.png` | [SVG](https://kassa.ai/assets/images/logo.svg) |
| `payment-cloudpayments.png` | [SVG с официального сайта CloudPayments](https://cdn.t-static.ru/static/pages/files/55eadcf3-411d-41f4-96ba-e893f9a4ca17.svg) |
